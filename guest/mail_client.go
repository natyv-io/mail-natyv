package main

import (
	"github.com/natyv-io/sdks/go/imap"
	"github.com/natyv-io/sdks/go/smtp"
)

const (
	imapHost = "imap.gmail.com"
	imapPort = 993
	smtpHost = "smtp.gmail.com"
	smtpPort = 465

	inboxFolder = "INBOX"
	// Gmail's real IMAP folder name for sent mail -- not just "Sent".
	sentFolder = "[Gmail]/Sent Mail"
)

var imapClient *imap.Client

func connectIMAP() error {
	cl, err := imap.Dial(imapHost, imapPort)
	if err != nil {
		return err
	}
	if err := cl.Login(gmailUsername, gmailAppPassword); err != nil {
		return err
	}
	imapClient = cl
	return nil
}

// withIMAPRetry runs attempt once against the current imapClient; on a
// real failure, reconnects once (a fresh imap.Dial+Login, replacing
// imapClient wholesale) and retries attempt exactly once more. Real,
// live-caught gap (2026-09-07): imapClient was previously assumed to stay
// valid for a guest instance's entire lifetime once connected (either at
// natyv_init or, after this ergonomics-layer retrofit, once per resume via
// rebuildApp's own connectIMAP call) -- but a real IMAP session can drop
// for reasons that have nothing to do with a recycle (a server-side idle
// timeout, a transient network blip), surfacing host-side as a real
// "unknown handle" error the moment a stale handle's connection is used
// again. Mirrors the phase2 spike's own already-established
// "reconnect-once-and-retry" contract for exactly this class of failure,
// generalized here since every real IMAP call site needs it, not just one.
func withIMAPRetry(attempt func() error) error {
	if err := attempt(); err != nil {
		if connErr := connectIMAP(); connErr != nil {
			return err
		}
		return attempt()
	}
	return nil
}

// listFolder selects mailbox and returns page's own message headers
// (lowest sequence number first -- callers wanting newest-first should
// reverse), pageSize messages per page. Page 0 is the pageSize most
// recent messages, page 1 the pageSize before that, and so on. canGoOlder
// reports whether any messages exist before this page's own range (i.e.
// whether a real "Older" page exists) -- computed straight from this same
// fetch's own real Exists/from, not a remembered total, so a real change
// in the mailbox's contents between page views is reflected immediately
// rather than compounding.
func listFolder(mailbox string, page, pageSize int) (msgs []imap.Message, canGoOlder bool, err error) {
	err = withIMAPRetry(func() error {
		info, serr := imapClient.Select(mailbox)
		if serr != nil {
			return serr
		}
		to := info.Exists - page*pageSize
		if to < 1 {
			msgs, canGoOlder = nil, false
			return nil
		}
		from := to - pageSize + 1
		if from < 1 {
			from = 1
		}
		fetched, ferr := imapClient.FetchHeaders(from, to)
		if ferr != nil {
			return ferr
		}
		msgs, canGoOlder = fetched, from > 1
		return nil
	})
	return
}

// readMessageBody re-selects currentFolder before fetching -- a folder
// view rendered from folderCache (a cache hit) never re-issues a real
// IMAP SELECT, so the connection's actually-selected mailbox can lag
// behind whatever folder the UI is currently showing; fetching a body
// against the wrong mailbox is what "imap: no such message" really means
// here, not a genuinely missing message (confirmed live, 2026-09-02).
func readMessageBody(seq int) (body string, err error) {
	err = withIMAPRetry(func() error {
		if _, serr := imapClient.Select(currentFolder); serr != nil {
			return serr
		}
		fetched, ferr := imapClient.FetchBody(seq)
		if ferr != nil {
			return ferr
		}
		body = fetched
		return nil
	})
	return
}

// deleteMessage removes seq from whichever mailbox is currently selected.
// Real caveat, inherited from imap.Client.Delete: against Gmail this
// archives rather than permanently deletes. Re-selects currentFolder on
// every attempt, not just the first -- a reconnect inside withIMAPRetry
// produces a brand-new connection with nothing selected at all, so a bare
// retry of Delete alone (relying on some earlier call's own Select still
// being in effect) would fail differently against a fresh connection.
func deleteMessage(seq int) error {
	return withIMAPRetry(func() error {
		if _, serr := imapClient.Select(currentFolder); serr != nil {
			return serr
		}
		return imapClient.Delete(seq)
	})
}

func sendMessage(to, subject, body string) error {
	cl, err := smtp.Dial(smtpHost, smtpPort, "mail-natyv")
	if err != nil {
		return err
	}
	defer cl.Quit()
	if err := cl.AuthLogin(gmailUsername, gmailAppPassword); err != nil {
		return err
	}
	return cl.Send(gmailUsername, to, subject, body)
}
