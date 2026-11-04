package library

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/store"
)

// What happens to a creator: read on the website and through the API, each kind
// able to be turned off. Failing to say so never undoes what happened.

// Mailer sends a notification by email. Nothing in this repository sets one yet:
// sending from production waits for its owner's decision on SMTP (docs/library.md),
// so with none there's no email, and the settings don't offer it.
type Mailer interface {
	Send(ctx context.Context, to, subject, text string) error
}

const (
	notificationsPage = 50
	mailTimeout       = 20 * time.Second
	mailsAtOnce       = 4
)

// notify keeps a notification for account unless it turned that kind off, and
// emails it if the account asked for that and this cloud can. Unread comments on
// a design say so once by email, however many more come.
func (s *Service) notify(ctx context.Context, account, kind, item, actor, reason string) {
	set, err := s.db.LibraryNotifySettings(ctx, account)
	if err != nil {
		s.log.Warn("library: notify", "err", err)
		return
	}
	if slices.Contains(set.Off, kind) {
		return
	}
	mail := s.cfg.Mailer != nil && set.Email
	if mail && kind == api.NotifyComment {
		said, err := s.db.LibraryNotifyUnreadOn(ctx, account, kind, item)
		mail = err == nil && !said
	}
	if err := s.db.AddLibraryNotification(ctx, account, kind, item, actor, reason); err != nil {
		s.log.Warn("library: notify", "err", err)
		return
	}
	if mail {
		go s.email(account, kind, item)
	}
}

// email says a notification by email, away from the request, the address read
// here and going nowhere else. A comment's text and a moderator's reason stay on
// the website: an inbox isn't asked to hold other people's words.
func (s *Service) email(account, kind, item string) {
	s.mails <- struct{}{}
	defer func() { <-s.mails }()
	ctx, cancel := context.WithTimeout(context.Background(), mailTimeout)
	defer cancel()
	acc, err := s.db.Account(ctx, account)
	if err != nil || acc.Email == "" {
		return
	}
	title := ""
	if it, err := s.db.LibraryItem(ctx, item); err == nil {
		title = it.Title
	}
	subject, text := mailOf(kind, title, strings.TrimSuffix(s.cfg.PublicURL, "/")+"/notifications")
	if err := s.cfg.Mailer.Send(ctx, acc.Email, subject, text); err != nil {
		s.log.Warn("library: email", "kind", kind)
	}
}

// mailOf is what an email says. A title can hold anything but a line break here,
// so a subject can't be made to add a header.
func mailOf(kind, title, link string) (subject, text string) {
	title = strings.Join(strings.Fields(title), " ")
	line := map[string]string{
		api.NotifyApproved:     "Your design was approved",
		api.NotifyRejected:     "Your design wasn't approved",
		api.NotifyRemoved:      "A moderator took something of yours down",
		api.NotifyComment:      "Someone commented on your design",
		api.NotifyReportUpheld: "A report on your design was upheld",
	}[kind]
	subject = "enowX UI: " + line
	return subject, fmt.Sprintf("%s: %s\n\nRead more: %s\n\nYou can turn these off in the same place.\n", line, title, link)
}

func (s *Service) listNotifications(w http.ResponseWriter, r *http.Request) {
	created, id, ok := decodeCursor(r.URL.Query().Get("cursor"))
	if !ok {
		fail(w, http.StatusBadRequest, "this cursor doesn't read")
		return
	}
	accID := asker(r).account.ID
	list, next, err := s.db.LibraryNotifications(r.Context(), accID, store.LibraryNotificationCursor{Created: created, ID: id}, notificationsPage)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	unread, err := s.db.LibraryUnread(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	reply(w, api.LibraryNotifications{Notifications: list, Unread: unread, Next: encodeCursor(next.Created, next.ID), Notice: api.LibraryDataNotice})
}

func (s *Service) readNotifications(w http.ResponseWriter, r *http.Request) {
	var p api.LibraryNotifyRead
	if !readJSON(w, r, &p) {
		return
	}
	if len(p.IDs) > notificationsPage {
		fail(w, http.StatusBadRequest, "that's more than a page of them")
		return
	}
	s.answer(w, nil, s.db.MarkLibraryNotificationsRead(r.Context(), asker(r).account.ID, p.IDs))
}

func (s *Service) notifySettings(w http.ResponseWriter, r *http.Request) {
	set, err := s.db.LibraryNotifySettings(r.Context(), asker(r).account.ID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	set.EmailAvailable = s.cfg.Mailer != nil
	reply(w, set)
}

func (s *Service) setNotifySettings(w http.ResponseWriter, r *http.Request) {
	var p api.LibraryNotifySettings
	if !readJSON(w, r, &p) {
		return
	}
	off := []string{}
	for _, kind := range p.Off {
		if !slices.Contains(api.LibraryNotifyKinds, kind) {
			fail(w, http.StatusBadRequest, "no such kind of notification")
			return
		}
		if !slices.Contains(off, kind) {
			off = append(off, kind)
		}
	}
	s.answer(w, nil, s.db.SetLibraryNotifySettings(r.Context(), asker(r).account.ID, api.LibraryNotifySettings{Off: off, Email: p.Email}))
}
