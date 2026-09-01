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

// growFixed is Width: Grow, Height: Fixed(height) -- the right shape for
// any leaf widget (Label/Button/TextField): Fit-sizing a leaf collapses it
// toward zero height, since natyv has no real font-driven text
// measurement yet (see the SDK's own Sizing.Fit doc comment).
func growFixed(parent uint32, height float32) widgets.Layout {
	l := widgets.ParentID(parent)
	l.Sizing = widgets.Sizing{Width: widgets.Grow(), Height: widgets.Fixed(height)}
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

	msgs, err := listFolder(folder, 20)
	if err != nil {
		return showError(err)
	}
	if len(msgs) == 0 {
		_, err := widgets.CreateLabel(growFixed(uint32(root), 24), "No messages.")
		return err
	}

	// Newest first.
	for i := len(msgs) - 1; i >= 0; i-- {
		if err := createMessageRow(uint32(root), msgs[i]); err != nil {
			return err
		}
	}
	return nil
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

	seq := m.Seq
	openBtn, err := widgets.CreateButton(growFixed(rowID, 28), "Open")
	if err != nil {
		return err
	}
	openBtn.OnClick(func() error { return showMessage(seq) })

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
		if err := sendMessage(to, subject, body); err != nil {
			return statusLbl.SetText("Error: " + err.Error())
		}
		return statusLbl.SetText("Sent!")
	})

	return nil
}

// -- Toolbar handlers, referenced by app.go.ntx's onClick={...} bindings --

func handleShowInbox() error   { return showFolder(inboxFolder) }
func handleShowSent() error    { return showFolder(sentFolder) }
func handleShowCompose() error { return showCompose() }
