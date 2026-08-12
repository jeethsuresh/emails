package calendar

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type caldavDiscoverRequest struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type caldavCalendarInfo struct {
	Path        string `json:"path"`
	DisplayName string `json:"displayName"`
	Color       string `json:"color,omitempty"`
}

type caldavSyncRequest struct {
	CalendarID string `json:"calendarId"`
	All        bool   `json:"all"`
}

func handleCalDAVDiscover(re *core.RequestEvent) error {
	var req caldavDiscoverRequest
	if err := re.BindBody(&req); err != nil {
		return re.BadRequestError("invalid json", err)
	}
	base := strings.TrimSpace(req.URL)
	user := strings.TrimSpace(req.Username)
	pass := req.Password
	if base == "" || user == "" {
		return re.BadRequestError("url and username required", nil)
	}
	cals, err := discoverCalDAVCalendars(base, user, pass)
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	return re.JSON(200, map[string]any{
		"ok":        true,
		"calendars": cals,
	})
}

func handleCalDAVSync(re *core.RequestEvent) error {
	var req caldavSyncRequest
	if err := re.BindBody(&req); err != nil {
		return re.BadRequestError("invalid json", err)
	}
	calCol, err := re.App.FindCollectionByNameOrId("calendars")
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}

	var targets []*core.Record
	if req.All {
		rows, err := re.App.FindRecordsByFilter(calCol.Id, "source = {:s}", "", 0, 0, dbx.Params{"s": "caldav"})
		if err != nil {
			return re.BadRequestError(err.Error(), err)
		}
		targets = rows
	} else {
		id := strings.TrimSpace(req.CalendarID)
		if id == "" {
			return re.BadRequestError("calendarId required", nil)
		}
		rec, err := re.App.FindRecordById(calCol, id)
		if err != nil {
			return re.BadRequestError("calendar not found", err)
		}
		targets = []*core.Record{rec}
	}

	results := make([]map[string]any, 0, len(targets))
	for _, cal := range targets {
		n, err := syncCalDAVCalendar(re.App, cal)
		item := map[string]any{"calendarId": cal.Id, "imported": n}
		if err != nil {
			cal.Set("last_error", err.Error())
			_ = re.App.Save(cal)
			item["error"] = err.Error()
		} else {
			cal.Set("last_error", "")
			cal.Set("last_sync_at", time.Now().UTC().Format(time.RFC3339))
			_ = re.App.Save(cal)
			item["ok"] = true
		}
		results = append(results, item)
	}
	return re.JSON(200, map[string]any{"ok": true, "results": results})
}

func discoverCalDAVCalendars(base, user, pass string) ([]caldavCalendarInfo, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	root, err := url.Parse(base)
	if err != nil {
		return nil, err
	}

	// Try well-known, then current URL as calendar-home-set probe.
	candidates := []string{
		strings.TrimRight(base, "/") + "/",
	}
	if root.Scheme != "" && root.Host != "" {
		candidates = append([]string{
			fmt.Sprintf("%s://%s/.well-known/caldav", root.Scheme, root.Host),
		}, candidates...)
	}

	var home string
	var lastErr error
	for _, cand := range candidates {
		principal, err := propfindCurrentUserPrincipal(client, cand, user, pass)
		if err != nil {
			lastErr = err
			continue
		}
		home, err = propfindCalendarHomeSet(client, resolveRef(cand, principal), user, pass)
		if err != nil {
			lastErr = err
			continue
		}
		break
	}
	if home == "" {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("could not discover calendar home")
	}

	return propfindCalendarList(client, resolveRef(base, home), user, pass)
}

func extractICSChunks(xmlBody string) []string {
	const begin = "BEGIN:VCALENDAR"
	const end = "END:VCALENDAR"
	var out []string
	rest := xmlBody
	for {
		i := strings.Index(rest, begin)
		if i < 0 {
			break
		}
		rest = rest[i:]
		j := strings.Index(rest, end)
		if j < 0 {
			break
		}
		chunk := rest[:j+len(end)]
		out = append(out, chunk)
		rest = rest[j+len(end):]
	}
	return out
}

func propfindCurrentUserPrincipal(client *http.Client, href, user, pass string) (string, error) {
	body := `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:">
  <d:prop><d:current-user-principal/></d:prop>
</d:propfind>`
	resp, err := doXML(client, "PROPFIND", href, user, pass, body, map[string]string{"Depth": "0"})
	if err != nil {
		return "", err
	}
	hrefOut := findXMLValue(resp, "href", "current-user-principal")
	if hrefOut == "" {
		return "", fmt.Errorf("no current-user-principal")
	}
	return hrefOut, nil
}

func propfindCalendarHomeSet(client *http.Client, href, user, pass string) (string, error) {
	body := `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:prop><c:calendar-home-set/></d:prop>
</d:propfind>`
	resp, err := doXML(client, "PROPFIND", href, user, pass, body, map[string]string{"Depth": "0"})
	if err != nil {
		return "", err
	}
	home := findXMLValue(resp, "href", "calendar-home-set")
	if home == "" {
		return "", fmt.Errorf("no calendar-home-set")
	}
	return home, nil
}

func propfindCalendarList(client *http.Client, home, user, pass string) ([]caldavCalendarInfo, error) {
	body := `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav" xmlns:cs="http://calendarserver.org/ns/">
  <d:prop>
    <d:displayname/>
    <d:resourcetype/>
    <cs:getctag/>
  </d:prop>
</d:propfind>`
	resp, err := doXML(client, "PROPFIND", home, user, pass, body, map[string]string{"Depth": "1"})
	if err != nil {
		return nil, err
	}
	return parseCalendarList(resp), nil
}

func reportCalendarData(href, user, pass string) (string, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	body := `<?xml version="1.0" encoding="utf-8"?>
<c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:prop>
    <d:getetag/>
    <c:calendar-data/>
  </d:prop>
  <c:filter>
    <c:comp-filter name="VCALENDAR">
      <c:comp-filter name="VEVENT"/>
    </c:comp-filter>
  </c:filter>
</c:calendar-query>`
	return doXML(client, "REPORT", href, user, pass, body, map[string]string{"Depth": "1"})
}

func doXML(client *http.Client, method, href, user, pass, body string, headers map[string]string) (string, error) {
	req, err := http.NewRequest(method, href, bytes.NewBufferString(body))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(user, pass)
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s %s: HTTP %d", method, href, resp.StatusCode)
	}
	return string(b), nil
}

func resolveRef(base, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return base
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	u, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return u.ResolveReference(r).String()
}

// findXMLValue walks tokens and returns the first nested <href> under a local-name match for parentHint.
func findXMLValue(doc, hrefLocal, parentLocal string) string {
	dec := xml.NewDecoder(strings.NewReader(doc))
	depthParent := -1
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		switch t := tok.(type) {
		case xml.StartElement:
			local := t.Name.Local
			if strings.EqualFold(local, parentLocal) {
				depthParent = 1
			} else if depthParent > 0 {
				depthParent++
				if strings.EqualFold(local, hrefLocal) {
					var href string
					_ = dec.DecodeElement(&href, &t)
					return strings.TrimSpace(href)
				}
			}
		case xml.EndElement:
			if depthParent > 0 {
				depthParent--
			}
		}
	}
}

func parseCalendarList(doc string) []caldavCalendarInfo {
	dec := xml.NewDecoder(strings.NewReader(doc))
	var out []caldavCalendarInfo
	var cur *caldavCalendarInfo
	var inResponse bool
	var isCalendar bool
	var displayName string
	var href string

	flush := func() {
		if inResponse && isCalendar && href != "" {
			name := displayName
			if name == "" {
				name = href
			}
			out = append(out, caldavCalendarInfo{Path: href, DisplayName: name})
		}
		cur = nil
		inResponse = false
		isCalendar = false
		displayName = ""
		href = ""
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			flush()
			return out
		}
		switch t := tok.(type) {
		case xml.StartElement:
			local := strings.ToLower(t.Name.Local)
			switch local {
			case "response":
				flush()
				inResponse = true
				_ = cur
			case "href":
				var v string
				_ = dec.DecodeElement(&v, &t)
				if href == "" {
					href = strings.TrimSpace(v)
				}
			case "displayname":
				var v string
				_ = dec.DecodeElement(&v, &t)
				displayName = strings.TrimSpace(v)
			case "calendar":
				isCalendar = true
			}
		case xml.EndElement:
			if strings.EqualFold(t.Name.Local, "response") {
				flush()
			}
		}
	}
}
