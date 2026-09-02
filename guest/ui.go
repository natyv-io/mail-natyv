package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/natyv-io/sdks/go/imap"
	"github.com/natyv-io/sdks/go/widgets"
)

// contentArea is bound by app.go.ntx's own ref={&contentArea} -- every
// dynamic view (inbox list, message read, compose) attaches under it.
// Pointer type: ref={&x} generates "x = &<createdVar>", so x itself must
// already be declared *widgets.Container for that assignment to type-check.
var contentArea *widgets.Container

// viewRoot is the current dynamic view's own single root Container --
// bound directly by each view composer's own top-level `ref={&viewRoot}`
// (views.go.ntx), replacing what a hand-written newViewRoot() used to do.
// Pointer type, same reason as contentArea above -- ref={&x} always
// generates "x = &<createdVar>". Swapped out on every view switch.
// Destroying it alone cascades through everything the view created
// (natyv-core's own destroy now cascades to every real Clay descendant),
// so no per-widget tracking is needed.
var viewRoot *widgets.Container

// toField/subjField/bodyField/statusLbl are bound by ComposeView's own
// ref={&x} attributes -- its Send button's handler (also written directly
// in views.go.ntx) reads/clears them by these same package-level names.
var toField *widgets.TextField
var subjField *widgets.TextField
var bodyField *widgets.TextArea
var statusLbl *widgets.Label

var currentFolder = inboxFolder

// pageSize is how many messages a folder page holds -- see listFolder's
// own doc comment for the paging scheme itself. One global setting shared
// by every folder (not per-folder), selectable at runtime via the
// page-size Dropdown in FolderView -- see setPageSize. Defaults to 5.
var pageSize = 5

// pageSizeOptions/pageSizeValues are FolderView's own page-size Dropdown
// choices -- parallel slices since Dropdown's real SDK options are always
// []string (the label shown per item), so the actual int each one means
// is looked up by the same index Dropdown's OnSelect hands back.
var pageSizeOptions = []string{"5", "10", "20"}
var pageSizeValues = []int{5, 10, 20}

// pageSizeLabel is FolderView's own Dropdown trigger text, built here
// (not inside views.go.ntx's markup) since natyv prepare's generated
// view body only ever imports the widgets package plus whatever a real
// `uses` declaration names -- never a plain import from the .ntx file's
// own source, which fmt.Sprintf would need.
func pageSizeLabel() string {
	return fmt.Sprintf("Page size: %d", pageSize)
}

// onPageSize is FolderView's own Dropdown onSelect handler -- maps the
// selected option's index back to the real int size it represents.
func onPageSize(index int) error {
	return setPageSize(pageSizeValues[index])
}

// folderDisplayName is FolderView's own friendly folder-name Label text --
// currentFolder's real value is a raw IMAP mailbox name ("[Gmail]/Sent
// Mail" for Sent), not something to show a user directly.
func folderDisplayName(folder string) string {
	switch folder {
	case inboxFolder:
		return "Inbox"
	case sentFolder:
		return "Sent"
	default:
		return folder
	}
}

// olderBtn/newerBtn are the currently-visible page's own "< "/"> " pager
// buttons -- single package-level slots, same reasoning as
// deleteSelectedBtn: only one folder page is ever actually visible (and
// thus receiving clicks) at a time. Enabled/disabled per page via
// Button.SetEnabled right after each is created (see FolderView's own
// markup) rather than hidden -- always visible so N (the current page)
// stays put instead of the whole pager shifting around.
var olderBtn *widgets.Button
var newerBtn *widgets.Button

// setPageSize changes how many messages a folder page holds and applies
// it immediately. Every folder's own already-cached pages and persisted
// view were built against the *old* page size, so "page 1" under the new
// size doesn't mean the same messages anymore -- all of it is dropped
// (every folder reset back to page 0) rather than trying to remap it, and
// whatever folder is currently on screen (if any) is rebuilt fresh.
func setPageSize(newSize int) error {
	pageSize = newSize
	folderCache = map[folderCacheKey]folderPageData{}
	for folder := range folderViews {
		destroyPersistedFolderView(folder)
	}
	for folder := range folderPage {
		folderPage[folder] = 0
	}
	if strings.HasPrefix(currentView, "folder:") {
		return renderFolder(currentFolder, 0, true)
	}
	return nil
}

// folderPage remembers each folder's own current page (0 = most recent),
// across navigation -- switching back to a folder shows whatever page it
// was last on, not always the most recent.
var folderPage = map[string]int{}

// folderCacheKey pairs a folder with a specific page -- each page is
// fetched and cached independently, since paging means there's no longer
// one single "this folder's message list" to cache per mailbox.
type folderCacheKey struct {
	folder string
	page   int
}

type folderPageData struct {
	msgs       []imap.Message
	canGoOlder bool
}

// folderCache holds each already-fetched (folder, page)'s own message
// list, keyed by folderCacheKey -- returning to a folder+page (the
// toolbar buttons, the read view's "< Back", or paging back to a page
// already visited) redisplays this instead of hitting IMAP again, since
// nothing server-side changes just by looking at a message. Invalidated
// explicitly wherever we know we've changed a folder's own contents (see
// invalidateFolder, called after a real send).
var folderCache = map[folderCacheKey]folderPageData{}

// cachedListFolder returns folderCache's entry for (folder, page) if
// present, otherwise fetches it for real via listFolder and caches the
// result.
func cachedListFolder(folder string, page int) (folderPageData, error) {
	key := folderCacheKey{folder, page}
	if data, ok := folderCache[key]; ok {
		return data, nil
	}
	msgs, canGoOlder, err := listFolder(folder, page, pageSize)
	if err != nil {
		return folderPageData{}, err
	}
	data := folderPageData{msgs: msgs, canGoOlder: canGoOlder}
	folderCache[key] = data
	return data, nil
}

// invalidateOtherPages drops every OTHER cached page of folder besides
// keepPage -- deleting a message on keepPage shifts every later message's
// real sequence number down by one (EXPUNGE's own behavior), which would
// silently desync any other already-cached page's Seq values otherwise.
// Simplest correct fix: forget them, so the next visit to one re-fetches
// for real instead of acting on stale sequence numbers.
func invalidateOtherPages(folder string, keepPage int) {
	for key := range folderCache {
		if key.folder == folder && key.page != keepPage {
			delete(folderCache, key)
		}
	}
}

// inboxRow is FolderView's own simplified per-row shape -- deliberately
// not imap.Message directly: natyv prepare's real codegen only ever
// forwards the widgets import into a generated file, never a composer's
// own signature-only imports, so a composer parameter typed []imap.Message
// fails to compile in the generated output (confirmed the hard way, a
// real gap worth a dedicated fix separately -- not a silent workaround
// papering over a design choice). Using a plain package-local type here
// needs no import at all, since views.go.ntx is the same package. Also a
// real simplification: the unread-indicator formatting moves here, next
// to the real imap.Message it reads, instead of living inside the
// <%...%> loop.
type inboxRow struct {
	From    string
	Subject string
	Seq     int
}

// truncateWithEllipsis clips s to at most maxRunes runes (never splitting a
// multi-byte UTF-8 sequence), appending "..." when it actually had to cut
// something. Real, disclosed mitigation for a real natyv limitation: Label
// has no font-driven text measurement and a fixed height that doesn't grow
// to fit wrapped text (see Breadcrumbs' own approxLabelWidth heuristic
// elsewhere in the SDK for the same underlying gap) -- a sender/subject
// long enough to wrap to a second line visibly overlaps whatever widget
// comes after it. Preventing the wrap in the first place, rather than
// trying to grow the box to fit it, is the only real fix available today.
func truncateWithEllipsis(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

func toInboxRows(msgs []imap.Message) []inboxRow {
	rows := make([]inboxRow, len(msgs))
	for i, msg := range msgs {
		from := msg.From
		if !msg.Seen {
			from = "* " + from
		}
		// Real headers can carry both a display name and a bracketed
		// address (e.g. `Google <no-reply@accounts.google.com>`) --
		// confirmed via a real click-through that this alone can exceed
		// one line's worth of characters even before truncation. Truncate
		// the *fully assembled* string, not a sub-piece before
		// concatenation -- truncating `msg.Subject` alone and appending
		// an unbounded date suffix afterward (an earlier, real bug in
		// this same function) just moves the overflow to a different
		// spot rather than fixing it.
		from = truncateWithEllipsis(from, 24)
		subject := truncateWithEllipsis(msg.Subject+"  ("+msg.Date+")", 26)
		rows[i] = inboxRow{From: from, Subject: subject, Seq: msg.Seq}
	}
	return rows
}

// smallFixed is a small, non-Grow fixed-size shape -- for a decorative
// widget like Spinner that shouldn't stretch to fill the row. Still real,
// hand-written Go (not `.ntx`): ComposeView's send handler creates the
// spinner dynamically, mid-event, not as part of the view's own initial
// markup, which `.ntx` doesn't have a construct for.
func smallFixed(parent uint32, width, height float32) widgets.Layout {
	l := widgets.ParentID(parent)
	l.Sizing = widgets.Sizing{Width: widgets.Fixed(width), Height: widgets.Fixed(height)}
	return l
}

// folderViews holds each already-built folder view's own persisted root
// Container, keyed by mailbox -- kept alive (hidden, never destroyed) once
// built, instead of being torn down and rebuilt every time that folder is
// shown again. Switching between Inbox and Sent (or back to either) is
// then a plain SetVisible flip, not a real widget-destroy-then-recreate
// pass -- confirmed live (2026-09-02) that the destroy+rebuild, however
// fast, produced a real, visible flash on every folder switch, not just a
// theoretical cost. Only ever rebuilt for real when we know its contents
// are actually stale (Refresh, or invalidateFolder after a send), or when
// its own page changes.
var folderViews = map[string]widgets.Container{}

// folderViewPage records which page each folder's own persisted view
// (folderViews) actually shows -- only ever one page kept alive per
// folder at a time, not every page ever visited, matching this app's
// existing memory-efficiency discipline. Paging within a folder always
// rebuilds, the same as Refresh.
var folderViewPage = map[string]int{}

// -- Row selection (checkbox + batch delete) --
//
// Real, deliberate v1 scope: selection only ever exists for whichever
// folder page is *currently on screen*. Switching away -- even back to a
// folder view that's kept alive and simply reused (folderViews) -- always
// clears it (see clearView's own "folder:" branch below), rather than
// tracking it per (folder, page) the way folderCache/folderViews do. This
// avoids two real problems for one deliberate cost: Seq numbers are only
// unique *within* one mailbox, so a bare int-keyed map shared across
// folders would collide (Inbox's seq 3 and Sent's seq 3 are unrelated
// messages); and a page whose own widgets are hidden, not destroyed,
// can't actually receive new clicks anyway, so nothing is lost by
// resetting its logical selection the moment it stops being the one
// visible page -- the checkboxes themselves are also explicitly reset to
// unchecked at the same time, so the visible state never lies about it.
var selectedSeqs = map[int]bool{}
var rowWidgets = map[int]widgets.Container{}
var rowCheckboxes = map[int]widgets.Checkbox{}

// deleteSelectedBtn is the currently-visible page's own "Delete Selected"
// button -- single package-level slot, same reasoning as viewRoot/toField
// above: only one folder page is ever actually visible (and thus
// receiving clicks) at a time.
var deleteSelectedBtn *widgets.Button

// registerRowWidget/registerRowCheckbox are called once per real
// MessageRow build (views.go.ntx), right after creating each -- lets this
// file act on a specific row later (checkbox toggle bookkeeping, direct
// destroy after a batch delete) without MessageRow's own signature
// needing to return anything beyond its already-real `error`.
func registerRowWidget(seq int, w widgets.Container)   { rowWidgets[seq] = w }
func registerRowCheckbox(seq int, cb widgets.Checkbox) { rowCheckboxes[seq] = cb }

// onRowSelect is MessageRow's own Checkbox onClick handler.
func onRowSelect(seq int, checked bool) error {
	if checked {
		selectedSeqs[seq] = true
	} else {
		delete(selectedSeqs, seq)
	}
	updateDeleteSelectedLabel()
	return nil
}

func updateDeleteSelectedLabel() {
	if deleteSelectedBtn == nil {
		return
	}
	_ = deleteSelectedBtn.SetLabel(fmt.Sprintf("Delete Selected (%d)", len(selectedSeqs)))
}

// resetSelectionState clears whatever's currently selected -- called
// whenever the currently-displayed folder page is about to stop being the
// one actually on screen (hidden-but-kept-alive) or is being rebuilt from
// scratch. uncheckWidgets is true only for the "hidden but still alive"
// case -- a page about to be destroyed outright has nothing left worth
// resetting.
func resetSelectionState(uncheckWidgets bool) {
	if uncheckWidgets {
		for _, cb := range rowCheckboxes {
			_ = cb.SetChecked(false)
		}
	}
	selectedSeqs = map[int]bool{}
	rowWidgets = map[int]widgets.Container{}
	rowCheckboxes = map[int]widgets.Checkbox{}
	updateDeleteSelectedLabel()
}

// onDeleteSelected is the toolbar's own single "Delete Selected" button
// handler -- deletes every currently-checked row in one batch. Processes
// highest Seq first: deleting a message shifts every later message's real
// sequence number down by one (EXPUNGE's own behavior, see
// removeAndShift's own doc comment), so deleting top-down keeps every
// still-pending lower Seq valid throughout the batch instead of racing
// its own earlier deletions.
func onDeleteSelected() error {
	seqs := make([]int, 0, len(selectedSeqs))
	for seq := range selectedSeqs {
		seqs = append(seqs, seq)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(seqs)))

	folder := currentFolder
	page := folderPage[folder]
	key := folderCacheKey{folder, page}
	cached := folderCache[key]
	for _, seq := range seqs {
		if err := deleteMessage(seq); err != nil {
			return showError(err)
		}
		cached.msgs = removeAndShift(cached.msgs, seq)
		if w, ok := rowWidgets[seq]; ok {
			w.Destroy()
		}
	}
	folderCache[key] = cached
	invalidateOtherPages(folder, page)

	selectedSeqs = map[int]bool{}
	rowWidgets = map[int]widgets.Container{}
	rowCheckboxes = map[int]widgets.Checkbox{}
	updateDeleteSelectedLabel()
	return nil
}

// currentView identifies whatever's actually rendered under viewRoot right
// now (e.g. "folder:INBOX", "message", "compose", "error") -- lets
// showFolder tell "already showing this" apart from "showing something
// else, or nothing yet", and lets clearView tell a persisted folder view
// (hide, don't destroy) apart from every other view (destroy as before).
// Every view-showing function below sets this itself, right alongside its
// own clearView() call, so it's never left stale by a transition this
// file doesn't know about.
var currentView string

// clearView retires the current view so a new one can take its place --
// cascades a real Destroy through every widget the view created, *unless*
// the current view is a persisted folder view (see folderViews above), in
// which case it's only hidden, so switching back to it later is instant.
func clearView() {
	if viewRoot == nil {
		return
	}
	if strings.HasPrefix(currentView, "folder:") {
		resetSelectionState(true)
		_ = viewRoot.SetVisible(false)
		viewRoot = nil
		return
	}
	viewRoot.Destroy()
	viewRoot = nil
}

func showError(err error) error {
	clearView()
	currentView = "error"
	return ErrorView(*contentArea, "Error: "+err.Error())
}

// -- Inbox / Sent list view --

// showFolder is the real entry point every caller outside this section
// uses (toolbar handlers, MessageDetailView's "< Back"). A no-op if we're
// already showing exactly this folder at its own remembered page.
// renderFolder does the actual work, and is also used directly by
// Refresh/paging, which must always force a real rebuild even when the
// folder itself isn't changing, since what it should show just did.
func showFolder(folder string) error {
	page := folderPage[folder]
	if currentView == "folder:"+folder && folderViewPage[folder] == page {
		return nil
	}
	return renderFolder(folder, page, false)
}

// destroyPersistedFolderView tears down folder's own persisted root (if
// any) for real -- shared by every renderFolder path that knows the
// persisted view can't just be reused (a different page, or forced).
func destroyPersistedFolderView(folder string) {
	root, ok := folderViews[folder]
	if !ok {
		return
	}
	root.Destroy()
	delete(folderViews, folder)
	delete(folderViewPage, folder)
	if viewRoot != nil && *viewRoot == root {
		viewRoot = nil
	}
}

func renderFolder(folder string, page int, forceRebuild bool) error {
	if root, ok := folderViews[folder]; ok && !forceRebuild && folderViewPage[folder] == page {
		// Already built, still fresh, and showing this exact page --
		// just swap it back in.
		clearView()
		currentFolder = folder
		currentView = "folder:" + folder
		viewRoot = &root
		return root.SetVisible(true)
	}
	destroyPersistedFolderView(folder)
	clearView()
	resetSelectionState(false)

	currentFolder = folder
	currentView = "folder:" + folder
	folderPage[folder] = page
	data, err := cachedListFolder(folder, page)
	if err != nil {
		return showError(err)
	}
	if err := FolderView(*contentArea, toInboxRows(data.msgs), folderDisplayName(folder), page, fmt.Sprintf("Page %d", page+1), data.canGoOlder, pageSizeLabel(),
		func() error { return renderFolder(folder, page-1, false) },
		func() error { return renderFolder(folder, page+1, false) },
		func() error {
			// Manual refresh -- Inbox's own cache is never auto-invalidated
			// by a send (unlike Sent, which we know for certain gets a new
			// entry), so this is the way to actually see new mail without
			// restarting the app.
			delete(folderCache, folderCacheKey{folder, page})
			return renderFolder(folder, page, true)
		},
		onPageSize,
		onDeleteSelected,
	); err != nil {
		return err
	}
	folderViews[folder] = *viewRoot
	folderViewPage[folder] = page
	updateDeleteSelectedLabel()
	return nil
}

// invalidateFolder drops every one of folder's cached pages and its own
// persisted view (if built), and resets it back to page 0 -- for the one
// case a folder's real contents change from *outside* its own Refresh
// button (a send always changes Sent's contents). Without dropping the
// persisted view too, the next visit would instantly swap back in a
// now-stale widget tree instead of rebuilding with fresh data.
func invalidateFolder(folder string) {
	for key := range folderCache {
		if key.folder == folder {
			delete(folderCache, key)
		}
	}
	folderPage[folder] = 0
	destroyPersistedFolderView(folder)
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

// -- Read view --

// headerFor looks up seq's real, untruncated From/Subject from
// currentFolder's own cached message list -- the same real imap.Message
// values toInboxRows reads, before that function's own display-only
// truncation (list rows are much narrower than the detail view, so
// reusing its truncated strings here would clip text that doesn't
// actually need clipping at this width).
func headerFor(seq int) (from, subject string) {
	data := folderCache[folderCacheKey{currentFolder, folderPage[currentFolder]}]
	for _, m := range data.msgs {
		if m.Seq == seq {
			return m.From, m.Subject
		}
	}
	return "", ""
}

func showMessage(seq int) error {
	body, err := readMessageBody(seq)
	if err != nil {
		return showError(err)
	}
	from, subject := headerFor(seq)
	clearView()
	currentView = "message"
	return MessageDetailView(*contentArea, truncateWithEllipsis(from, 100), truncateWithEllipsis(subject, 100), body, func() error { return showFolder(currentFolder) })
}

// -- Compose view --

func showCompose() error {
	clearView()
	currentView = "compose"
	return ComposeView(*contentArea)
}

// -- Toolbar handlers, referenced by app.go.ntx's onClick={...} bindings --

func handleShowInbox() error   { return showFolder(inboxFolder) }
func handleShowSent() error    { return showFolder(sentFolder) }
func handleShowCompose() error { return showCompose() }
