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

// listFolder selects mailbox and returns up to the maxCount most recent
// message headers (lowest sequence number first -- callers wanting
// newest-first should reverse).
func listFolder(mailbox string, maxCount int) ([]imap.Message, error) {
	info, err := imapClient.Select(mailbox)
	if err != nil {
		return nil, err
	}
	if info.Exists == 0 {
		return nil, nil
	}
	from := info.Exists - maxCount + 1
	if from < 1 {
		from = 1
	}
	return imapClient.FetchHeaders(from, info.Exists)
}

func readMessageBody(seq int) (string, error) {
	return imapClient.FetchBody(seq)
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
