package calendar

import (
	"github.com/pocketbase/pocketbase/core"
)

// Register wires calendar HTTP routes (window projection, writes, ICS, CalDAV, settings).
func Register(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.GET("/api/email/calendar/settings", handleGetSettings)
		e.Router.POST("/api/email/calendar/settings", handlePostSettings)

		e.Router.GET("/api/email/calendar/window", handleGetWindow)
		e.Router.GET("/api/email/calendar/bounds", handleGetBounds)

		e.Router.POST("/api/email/calendar/events", handleCreateEvent)
		e.Router.PATCH("/api/email/calendar/events/{id}", handleUpdateEvent)

		e.Router.POST("/api/email/calendar/calendars/local", handleCreateLocalCalendar)

		e.Router.POST("/api/email/calendar/ics/import", handleICSImport)
		e.Router.GET("/api/email/calendar/ics/export", handleICSExport)
		e.Router.POST("/api/email/calendar/ics/refresh", handleICSRefresh)

		e.Router.POST("/api/email/calendar/caldav/discover", handleCalDAVDiscover)
		e.Router.POST("/api/email/calendar/caldav/subscribe", handleCalDAVSubscribe)
		e.Router.POST("/api/email/calendar/caldav/sync", handleCalDAVSync)
		e.Router.POST("/api/email/calendar/sync", handleCalendarSyncAll)

		StartSyncLoop(app)
		return e.Next()
	})
}
