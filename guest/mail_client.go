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
	info, err := imapClient.Select(mailbox)
	if err != nil {
		return nil, false, err
	}
	to := info.Exists - page*pageSize
	if to < 1 {
		return nil, false, nil
	}
	from := to - pageSize + 1
	if from < 1 {
		from = 1
	}
	msgs, err = imapClient.FetchHeaders(from, to)
	if err != nil {
		return nil, false, err
	}
	return msgs, from > 1, nil
}

// readMessageBody re-selects currentFolder before fetching -- a folder
// view rendered from folderCache (a cache hit) never re-issues a real
// IMAP SELECT, so the connection's actually-selected mailbox can lag
// behind whatever folder the UI is currently showing; fetching a body
// against the wrong mailbox is what "imap: no such message" really means
// here, not a genuinely missing message (confirmed live, 2026-09-02).
func readMessageBody(seq int) (string, error) {
	if _, err := imapClient.Select(currentFolder); err != nil {
		return "", err
	}
	return imapClient.FetchBody(seq)
}

// deleteMessage removes seq from whichever mailbox is currently selected.
// Real caveat, inherited from imap.Client.Delete: against Gmail this
// archives rather than permanently deletes.
func deleteMessage(seq int) error {
	return imapClient.Delete(seq)
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
