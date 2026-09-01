package main

import (
	"github.com/natyv-io/sdks/go/imap"
	"github.com/natyv-io/sdks/go/widgets"
)

// contentArea is bound by app.go.ntx's own ref={&contentArea} -- every
// dynamic view (inbox list, message read, compose) attaches under it.
// Pointer type: ref={&x} generates "x = &<createdVar>", so x itself must
// already be declared *widgets.Container for that assignment to type-check.
var contentArea *widgets.Container

// viewRoot is the current dynamic view's own single root Container --
// swapped out on every view switch. Destroying it alone cascades through
// everything the view created (natyv-core's own destroy now cascades to
// every real Clay descendant), so no per-widget tracking is needed.
var viewRoot widgets.Container

var currentFolder = inboxFolder

// folderCache holds the last-fetched message list per folder, keyed by
// mailbox name -- returning to a folder (the toolbar buttons, or the read
// view's "< Back") redisplays this instead of hitting IMAP again, since
// nothing server-side changes just by looking at a message. Invalidated
// explicitly wherever we know we've changed a folder's own contents (see
// sendMessage's own caller in showCompose, which drops sentFolder's
// cache entry after a real send so the next visit re-fetches for real).
var folderCache = map[string][]imap.Message{}

// cachedListFolder returns folderCache's entry for mailbox if present,
// otherwise fetches it for real via listFolder and caches the result.
func cachedListFolder(mailbox string, maxCount int) ([]imap.Message, error) {
	if msgs, ok := folderCache[mailbox]; ok {
		return msgs, nil
	}
	msgs, err := listFolder(mailbox, maxCount)
	if err != nil {
		return nil, err
	}
	folderCache[mailbox] = msgs
	return msgs, nil
}

// growFixed is Width: Grow, Height: Fixed(height) -- the right shape for
// any leaf widget (Label/Button/TextField): Fit-sizing a leaf collapses it
// toward zero height, since natyv has no real font-driven text
// measurement yet (see the SDK's own Sizing.Fit doc comment).
func growFixed(parent uint32, height float32) widgets.Layout {
	l := widgets.ParentID(parent)
	l.Sizing = widgets.Sizing{Width: widgets.Grow(), Height: widgets.Fixed(height)}
	return l
}

// smallFixed is a small, non-Grow fixed-size shape -- for a decorative
// widget like Spinner that shouldn't stretch to fill the row.
func smallFixed(parent uint32, width, height float32) widgets.Layout {
	l := widgets.ParentID(parent)
	l.Sizing = widgets.Sizing{Width: widgets.Fixed(width), Height: widgets.Fixed(height)}
	return l
}

// growBoth is Width: Grow, Height: Grow -- for a body TextArea that should
// fill whatever vertical space is left in the view.
func growBoth(parent uint32) widgets.Layout {
	l := widgets.ParentID(parent)
	l.Sizing = widgets.Sizing{Width: widgets.Grow(), Height: widgets.Grow()}
	return l
}

// clearView destroys the current view's root (if any) -- cascades through
// every widget the view created.
func clearView() {
	if viewRoot != 0 {
		viewRoot.Destroy()
		viewRoot = 0
	}
}

// newViewRoot creates a fresh TopToBottom root under contentArea, tracked
// as the current view for the next clearView() call.
func newViewRoot() (widgets.Container, error) {
	l := widgets.ParentID(uint32(*contentArea))
	l.Direction = widgets.TopToBottom
	l.ChildGap = 8
	l.Sizing = widgets.Sizing{Width: widgets.Grow(), Height: widgets.Grow()}
	root, err := widgets.CreateContainer(l, false, 0)
	if err != nil {
		return 0, err
	}
	viewRoot = root
	return root, nil
}

func showError(err error) error {
	clearView()
	root, rerr := newViewRoot()
	if rerr != nil {
		return rerr
	}
	_, lerr := widgets.CreateLabel(growFixed(uint32(root), 24), "Error: "+err.Error())
	return lerr
}

// -- Inbox / Sent list view --

func showFolder(folder string) error {
	clearView()
	currentFolder = folder

	root, err := newViewRoot()
	if err != nil {
		return err
	}
	rootID := uint32(root)

	// Manual refresh -- Inbox's own cache is never auto-invalidated by a
	// send (unlike Sent, which we know for certain gets a new entry), so
	// this is the way to actually see new mail without restarting the app.
	refreshBtn, err := widgets.CreateButton(growFixed(rootID, 28), "Refresh")
	if err != nil {
		return err
	}
	refreshBtn.OnClick(func() error {
		delete(folderCache, folder)
		return showFolder(folder)
	})

	msgs, err := cachedListFolder(folder, 20)
	if err != nil {
		return showError(err)
	}
	if len(msgs) == 0 {
		_, err := widgets.CreateLabel(growFixed(rootID, 24), "No messages.")
		return err
	}

	// Newest first.
	for i := len(msgs) - 1; i >= 0; i-- {
		if err := createMessageRow(rootID, msgs[i]); err != nil {
			return err
		}
	}
	return nil
}

// removeAndShift drops the message whose sequence number is deletedSeq and
// corrects every remaining message's own Seq to match what the server's
// real post-EXPUNGE state would be -- EXPUNGE shifts every message with a
// higher sequence number down by one, so leaving them untouched would make
// a later action against one of them (Open, Delete) act on the wrong
// message. Filters in place (reuses msgs' own backing array) since this
// is always called with the caller's own already-owned cache slice, never
// a shared one.
func removeAndShift(msgs []imap.Message, deletedSeq int) []imap.Message {
	updated := msgs[:0]
	for _, msg := range msgs {
		switch {
		case msg.Seq == deletedSeq:
			continue
		case msg.Seq > deletedSeq:
			msg.Seq--
		}
		updated = append(updated, msg)
	}
	return updated
}

func createMessageRow(parent uint32, m imap.Message) error {
	rowLayout := widgets.ParentID(parent)
	rowLayout.Direction = widgets.TopToBottom
	rowLayout.ChildGap = 2
	rowLayout.Sizing = widgets.Sizing{Width: widgets.Grow(), Height: widgets.Fit()}
	row, err := widgets.CreateContainer(rowLayout, false, 0)
	if err != nil {
		return err
	}
	rowID := uint32(row)

	fromText := m.From
	if !m.Seen {
		fromText = "* " + fromText
	}
	if _, err := widgets.CreateLabel(growFixed(rowID, 20), fromText); err != nil {
		return err
	}
	if _, err := widgets.CreateLabel(growFixed(rowID, 20), m.Subject+"  ("+m.Date+")"); err != nil {
		return err
	}

	btnRowLayout := widgets.ParentID(rowID)
	btnRowLayout.Direction = widgets.LeftToRight
	btnRowLayout.ChildGap = 4
	btnRowLayout.Sizing = widgets.Sizing{Width: widgets.Grow(), Height: widgets.Fit()}
	btnRow, err := widgets.CreateContainer(btnRowLayout, false, 0)
	if err != nil {
		return err
	}
	btnRowID := uint32(btnRow)

	seq := m.Seq
	openBtn, err := widgets.CreateButton(growFixed(btnRowID, 28), "Open")
	if err != nil {
		return err
	}
	openBtn.OnClick(func() error { return showMessage(seq) })

	deleteBtn, err := widgets.CreateButton(growFixed(btnRowID, 28), "Delete")
	if err != nil {
		return err
	}
	folder := currentFolder
	deleteBtn.OnClick(func() error {
		if err := deleteMessage(seq); err != nil {
			return showError(err)
		}
		// Surgical update, no re-fetch -- see removeAndShift's own doc
		// comment for why the other cached entries need correcting too,
		// not just the deleted one dropped.
		if cached, ok := folderCache[folder]; ok {
			folderCache[folder] = removeAndShift(cached, seq)
		}
		row.Destroy()
		return nil
	})

	return nil
}

// -- Read view --

func showMessage(seq int) error {
	body, err := readMessageBody(seq)
	if err != nil {
		return showError(err)
	}

	clearView()
	root, err := newViewRoot()
	if err != nil {
		return err
	}
	rootID := uint32(root)

	backBtn, err := widgets.CreateButton(growFixed(rootID, 28), "< Back")
	if err != nil {
		return err
	}
	backBtn.OnClick(func() error { return showFolder(currentFolder) })

	bodyArea, err := widgets.CreateTextArea(growBoth(rootID), "")
	if err != nil {
		return err
	}
	return bodyArea.SetText(decodeMimeBody(body))
}

// -- Compose view --

func showCompose() error {
	clearView()
	root, err := newViewRoot()
	if err != nil {
		return err
	}
	rootID := uint32(root)

	toField, err := widgets.CreateTextField(growFixed(rootID, 28), "To")
	if err != nil {
		return err
	}
	subjField, err := widgets.CreateTextField(growFixed(rootID, 28), "Subject")
	if err != nil {
		return err
	}
	bodyField, err := widgets.CreateTextArea(growBoth(rootID), "Body")
	if err != nil {
		return err
	}
	statusLbl, err := widgets.CreateLabel(growFixed(rootID, 20), "")
	if err != nil {
		return err
	}
	sendBtn, err := widgets.CreateButton(growFixed(rootID, 28), "Send")
	if err != nil {
		return err
	}
	sendBtn.OnClick(func() error {
		to, _ := toField.Text()
		subject, _ := subjField.Text()
		body, _ := bodyField.Text()

		if err := statusLbl.SetText(""); err != nil {
			return err
		}
		// Real, deliberate visual feedback during the blocking send call:
		// this natyv_dispatch handler runs on the worker thread and blocks
		// for the real network round trip, but widget creation itself
		// mutates the shared registry immediately (before that blocking
		// call runs) -- SDL's own render loop, on the separate main
		// thread, keeps drawing independently the whole time, so the
		// spinner genuinely animates live during the send, not just at
		// the very end.
		spinner, serr := widgets.CreateSpinner(smallFixed(rootID, 60, 16))
		if serr != nil {
			return serr
		}
		sendErr := sendMessage(to, subject, body)
		spinner.Destroy()

		if sendErr != nil {
			return statusLbl.SetText("Error: " + sendErr.Error())
		}

		// A real send always changes Sent's own contents -- drop its cache
		// entry so the next visit re-fetches for real. Inbox is left
		// alone even though a self-send would technically affect it too:
		// invalidating it on every send would mean paying a real refetch
		// for the common case (sending to someone else) just to handle a
		// rare one -- the Refresh button in the folder view is the real,
		// explicit way to see new mail there.
		delete(folderCache, sentFolder)

		if err := toField.Clear(); err != nil {
			return err
		}
		if err := subjField.Clear(); err != nil {
			return err
		}
		if err := bodyField.Clear(); err != nil {
			return err
		}

		return statusLbl.SetText("Sent!")
	})

	return nil
}

// -- Toolbar handlers, referenced by app.go.ntx's onClick={...} bindings --

func handleShowInbox() error   { return showFolder(inboxFolder) }
func handleShowSent() error    { return showFolder(sentFolder) }
func handleShowCompose() error { return showCompose() }
