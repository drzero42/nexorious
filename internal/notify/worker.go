package notify

import (
	"context"
	"log/slog"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/crypto"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
)

// NotifyWorker delivers one event to all subscribed recipients' channels.
type NotifyWorker struct {
	river.WorkerDefaults[NotifyArgs]
	DB        *bun.DB
	Encrypter *crypto.Encrypter
	Sender    Sender
}

// Work loads the event, resolves recipients, renders, and sends. Per-channel
// failures are logged and swallowed; missing/unreadable events are not retried.
func (w *NotifyWorker) Work(ctx context.Context, job *river.Job[NotifyArgs]) error {
	var ev models.Event
	if err := w.DB.NewSelect().Model(&ev).Where("id = ?", job.Args.EventID).Scan(ctx); err != nil {
		slog.WarnContext(ctx, "notify: load event", "event_id", job.Args.EventID, logging.KeyErr, err, logging.KeyCategory, logging.CategoryDB)
		return nil
	}

	recipients, err := w.resolveRecipients(ctx, &ev)
	if err != nil {
		slog.ErrorContext(ctx, "notify: resolve recipients", "event_id", ev.ID, "type", ev.Type, logging.KeyErr, err, logging.KeyCategory, logging.CategoryDB)
		return nil
	}

	title, body, derr := Format(ev.Type, ev.Payload)
	if derr != nil {
		slog.WarnContext(ctx, "notify: payload decode failed", "event_id", ev.ID, "type", ev.Type, logging.KeyErr, derr)
	}

	for _, userID := range recipients {
		channels, err := w.loadChannels(ctx, userID)
		if err != nil {
			slog.WarnContext(ctx, "notify: load channels", logging.KeyUserID, userID, logging.KeyErr, err, logging.KeyCategory, logging.CategoryDB)
			continue
		}
		for _, ch := range channels {
			plain, cerr := w.Encrypter.Decrypt(ch.EncryptedURL)
			if cerr != nil {
				slog.WarnContext(ctx, "notify: decrypt channel url", "channel_id", ch.ID, logging.KeyErr, cerr)
				continue
			}
			if serr := w.Sender.Send(ctx, string(plain), title, body); serr != nil {
				slog.WarnContext(ctx, "notify: send", "channel_id", ch.ID, "type", ev.Type, logging.KeyErr, serr, logging.KeyCategory, logging.CategoryExternalAPI)
				continue
			}
			slog.DebugContext(ctx, "notify: sent", "channel_id", ch.ID, "type", ev.Type)
		}
	}
	return nil
}

// resolveRecipients returns the user IDs to deliver to.
func (w *NotifyWorker) resolveRecipients(ctx context.Context, ev *models.Event) ([]string, error) {
	if ev.Scope == ScopeAdmin {
		var rows []struct {
			ID string `bun:"id"`
		}
		err := w.DB.NewRaw(
			`SELECT u.id FROM users u
			   JOIN notification_subscriptions s ON s.user_id = u.id
			  WHERE u.is_admin = true AND s.event_type = ?`,
			ev.Type,
		).Scan(ctx, &rows)
		if err != nil {
			return nil, err
		}
		ids := make([]string, len(rows))
		for i, r := range rows {
			ids[i] = r.ID
		}
		return ids, nil
	}
	if ev.ActorUserID == nil {
		return nil, nil
	}
	var rows []struct {
		UserID string `bun:"user_id"`
	}
	err := w.DB.NewRaw(
		`SELECT user_id FROM notification_subscriptions WHERE user_id = ? AND event_type = ?`,
		*ev.ActorUserID, ev.Type,
	).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.UserID
	}
	return ids, nil
}

func (w *NotifyWorker) loadChannels(ctx context.Context, userID string) ([]models.NotificationChannel, error) {
	var channels []models.NotificationChannel
	err := w.DB.NewSelect().Model(&channels).Where("user_id = ?", userID).Order("created_at").Scan(ctx)
	return channels, err
}
