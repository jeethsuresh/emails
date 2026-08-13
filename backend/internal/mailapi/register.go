package mailapi

import "github.com/pocketbase/pocketbase/core"

// Register wires the mail list, compose, and send HTTP routes.
func Register(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.GET("/api/email/threads", handleListThreads)
		e.Router.GET("/api/email/threads/{id}", handleGetThread)
		e.Router.GET("/api/email/aliases", handleAliases)
		e.Router.GET("/api/email/contacts", handleContacts)
		e.Router.GET("/api/email/contacts/{email}/messages", handleContactMessages)
		e.Router.POST("/api/email/compose/reply", handleComposeReply)
		e.Router.POST("/api/email/send", handleSend)
		return e.Next()
	})
}
