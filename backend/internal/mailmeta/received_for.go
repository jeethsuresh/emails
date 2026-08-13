package mailmeta

import "strings"

func ReceivedFor(headers map[string]string, toAddrs, ccAddrs, accountEmail string) string {
	for _, key := range []string{"Delivered-To", "X-Original-To", "X-Delivered-To", "Envelope-To"} {
		if addrs := ParseAddressList(headerGet(headers, key)); len(addrs) > 0 {
			return addrs[0]
		}
	}
	domain := DomainOf(accountEmail)
	candidates := append(ParseAddressList(toAddrs), ParseAddressList(ccAddrs)...)
	for _, c := range candidates {
		if domain != "" && DomainOf(c) == domain {
			return c
		}
		if NormalizeEmail(c) == NormalizeEmail(accountEmail) {
			return c
		}
	}
	return ""
}

func headerGet(h map[string]string, name string) string {
	if v, ok := h[name]; ok {
		return v
	}
	for k, v := range h {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}
