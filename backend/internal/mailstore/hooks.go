package mailstore

import (
	"email.local/backend/internal/mailmeta"

	"github.com/pocketbase/pocketbase/core"
)

func bindMessageThreadHooks(app core.App) {
	app.OnRecordAfterCreateSuccess("messages").BindFunc(func(e *core.RecordEvent) error {
		if err := mailmeta.UpsertThreadFromMessage(e.App, e.Record); err != nil {
			return err
		}
		return e.Next()
	})
	app.OnRecordAfterUpdateSuccess("messages").BindFunc(func(e *core.RecordEvent) error {
		if err := mailmeta.UpsertThreadFromMessage(e.App, e.Record); err != nil {
			return err
		}
		return e.Next()
	})
	app.OnRecordAfterDeleteSuccess("messages").BindFunc(func(e *core.RecordEvent) error {
		if err := mailmeta.RecountThread(e.App, e.Record.GetString("thread_id")); err != nil {
			return err
		}
		return e.Next()
	})
}
