package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	natyv "github.com/natyv-io/sdks/go"
	"github.com/natyv-io/sdks/go/imap"
	"github.com/natyv-io/sdks/go/widgets"
)

// appRoot is this app's own top-level Container (App's own parent,
// created once in natyv_init) -- the stable anchor App/contentArea/every
// dynamic view attaches under. Promoted from a
// natyv_init-local var to package-level and persisted (via
// persistedAppRoot, natyv_resume's Get() call restores this before
// rebuildApp ever runs) so a resumed instance's own Go runtime (which
// forgets every package-level value on a fresh instantiation, even though
// the *host* never destroyed the underlying widget) can still name the
// exact same real widget id later navigation needs. Named appRoot, not
// root, specifically to avoid colliding with renderFolder's/
// destroyPersistedFolderView's own pre-existing local `root` (a folder's
// persisted view Container) -- Go would silently let a real reference
// here resolve to the wrong one if they shared a name.
var appRoot widgets.Container
var persistedAppRoot = natyv.Persisted[widgets.Container]("appRoot", 0)

// appRootLayout is the real Layout appRoot itself is created with (see
// main.go's natyvInit). appRoot only ever has one direct child
// (natyvBuildApp's own Container0), so Direction is moot today, but this
// still mirrors appRoot's real creation Layout rather than assuming that
// stays true forever.
var appRootLayout = widgets.Layout{Sizing: widgets.Sizing{Width: widgets.Grow(), Height: widgets.Grow()}}

// contentArea is bound by app.go.ntx's own ref={&contentArea} -- every
// dynamic view (inbox list, message read, compose) attaches under it.
// Pointer type: ref={&x} generates "x = &<createdVar>", so x itself must
// already be declared *widgets.Container for that assignment to type-check.
//
// Part 2 (codegen automation) regen, 2026-09-11: persistence used to be
// hand-batched here (Mechanism 1's own manual precursor) -- now automatic,
// natyv_generated.go's own genSnapshotRefs/genRestoreRefs (called via
// GeneratedCheckpoint/GeneratedResume, main.go) handle every ref='d
// non-struct-backed widget in this file unconditionally, this one
// included, with no hand-written Set/Get needed anymore.
var contentArea *widgets.Container

// viewRoot is the current dynamic view's own single root Container --
// bound directly by each view composer's own top-level `ref={&viewRoot}`
// (views.go.ntx), replacing what a hand-written newViewRoot() used to do.
// Pointer type, same reason as contentArea above -- ref={&x} always
// generates "x = &<createdVar>". Swapped out on every view switch.
// Destroying it alone cascades through everything the view created
// (natyv-core's own destroy now cascades to every real Clay descendant),
// so no per-widget tracking is needed. Persistence is automatic now, same
// as contentArea above.
var viewRoot *widgets.Container

// toField/subjField/bodyField/statusLbl are bound by ComposeView's own
// ref={&x} attributes -- handleComposeSend (below) reads/clears them by
// these same package-level names. Persistence is automatic now, same as
// contentArea/viewRoot above.
var toField *widgets.TextField
var subjField *widgets.TextField
var bodyField *widgets.TextArea
var statusLbl *widgets.Label

// handleComposeSend is ComposeView's own Send button handler, extracted
// to a real named function (2026-09-10, resume-without-recreate plan,
// step 5) so both the real .ntx build (onClick={handleComposeSend}) and
// natyv_resume's own rebindComposeSendClick can reference the identical
// logic -- same precedent as handleShowInbox/handleShowSent/
// handleShowCompose, all already plain named functions for exactly this
// reason.
func handleComposeSend() error {
	to, _ := toField.Text()
	subject, _ := subjField.Text()
	body, _ := bodyField.Text()

	if err := statusLbl.SetText(""); err != nil {
		return err
	}
	// Real, deliberate visual feedback during the blocking send call: this
	// natyv_dispatch handler runs on the worker thread and blocks for the
	// real network round trip, but widget creation itself mutates the
	// shared registry immediately (before that blocking call runs) --
	// SDL's own render loop, on the separate main thread, keeps drawing
	// independently the whole time, so the spinner genuinely animates live
	// during the send, not just at the very end.
	spinner, serr := widgets.CreateSpinner(smallFixed(uint32(*viewRoot), 60, 16))
	if serr != nil {
		return serr
	}
	sendErr := sendMessage(to, subject, body)
	spinner.Destroy()

	if sendErr != nil {
		return statusLbl.SetText("Error: " + sendErr.Error())
	}

	// A real send always changes Sent's own contents -- drop its cache
	// entry (and its persisted view, if built) so the next visit rebuilds
	// with fresh data instead of reusing a now-stale one. Inbox is left
	// alone even though a self-send would technically affect it too:
	// invalidating it on every send would mean paying a real refetch for
	// the common case (sending to someone else) just to handle a rare one
	// -- the Refresh button in the folder view is the real, explicit way
	// to see new mail there.
	invalidateFolder(sentFolder)

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
}

// currentFolder is the raw IMAP mailbox name currently showing -- persisted
// via persistedCurrentFolder so a resumed instance's rebuildApp (below)
// knows which folder to re-render, not just that some folder was open.
var currentFolder = inboxFolder
var persistedCurrentFolder = natyv.Persisted[string]("currentFolder", inboxFolder)

// pageSize is how many messages a folder page holds -- see listFolder's
// own doc comment for the paging scheme itself. One global setting shared
// by every folder (not per-folder), selectable at runtime via the
// page-size Dropdown in FolderView -- see setPageSize. Defaults to 5;
// persisted via persistedPageSize so a resumed instance keeps whatever
// size the user last chose.
var pageSize = 5
var persistedPageSize = natyv.Persisted[int]("pageSize", 5)

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

// olderBtn/newerBtn/rowsContainer/pagerLabelWidget/rowWidget are all
// single package-level ref slots -- only one FolderView/MessageRow build
// is ever in flight at a time, so a shared slot captured immediately
// after creation (FolderView's own trailing onBuilt callback, or
// registerRowWidget) is safe, same reasoning as deleteSelectedBtn below
// and viewRoot/contentArea above. Enabled/disabled per page via
// Button.SetEnabled right after each pager button is created (see
// FolderView's own markup) rather than hidden -- always visible so N (the
// current page) stays put instead of the whole pager shifting around.
// rowsContainer is the persisted parent navigatePage rebuilds message
// rows under; pagerLabelWidget lets navigatePage update the "Page N" text
// in place; rowWidget is MessageRow's own top-level row Container (see
// its own doc comment for why it doesn't need a true per-invocation local
// the way its Checkbox's onClick closure does).
var olderBtn *widgets.Button
var newerBtn *widgets.Button
var rowsContainer *widgets.Container
var pagerLabelWidget *widgets.Label
var rowWidget *widgets.Container
var deleteSelectedBtn *widgets.Button

// Part 2 (codegen automation) regen, 2026-09-11: inboxBtn/sentBtn/
// composeBtn/refreshBtn/backBtn/sendBtn/pageSizeDropdown (the vars this
// comment used to document) are gone -- every one of them existed purely
// so a RegisterBinding call elsewhere in this file had a widget id to
// bind against, and that's now handled directly by the generated
// RegisterBinding call at each tag's own creation site (auto for nav_*/
// compose_send_click, via bindKind=/bindArgs= for refresh_click/
// back_click/page_size_change) -- no ref= or package-level slot needed
// for that purpose anymore.

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
// was last on, not always the most recent. Persisted via
// persistedFolderPage so a resumed instance keeps every folder's own page
// position, not just the one currently on screen.
var folderPage = map[string]int{}
var persistedFolderPage = natyv.Persisted[map[string]int]("folderPage", map[string]int{})

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
var persistedFolderViews = natyv.Persisted[map[string]widgets.Container]("folderViews", map[string]widgets.Container{})

// folderViewPage records which page each folder's own persisted view
// (folderViews) actually shows -- only ever one page kept alive per
// folder at a time, not every page ever visited, matching this app's
// existing memory-efficiency discipline. Paging within a folder always
// rebuilds, the same as Refresh.
var folderViewPage = map[string]int{}
var persistedFolderViewPage = natyv.Persisted[map[string]int]("folderViewPage", map[string]int{})

// -- Row selection (checkbox + batch delete) --
//
// Real, deliberate v1 scope: selection only ever exists for whichever
// folder page is *currently on screen*, kept inside that folder's own
// *folderUI rather than a shared package-level map -- Seq numbers are
// only unique *within* one mailbox, so a bare int-keyed map shared across
// folders would collide (Inbox's seq 3 and Sent's seq 3 are unrelated
// messages).

// folderUI holds one folder's own live widget handles and row-selection
// state -- everything navigatePage/onRowSelect/onDeleteSelected need in
// order to act on "whichever folder is actually on screen." Exactly one
// *folderUI is ever active (pointed to by `active`) at a time, matching
// this app's existing single-visible-page assumption everywhere else.
type folderUI struct {
	rowsContainer widgets.Container
	olderBtn      widgets.Button
	newerBtn      widgets.Button
	pageLabel     widgets.Label
	deleteBtn     widgets.Button

	selectedSeqs  map[int]bool
	rowWidgets    map[int]widgets.Container
	rowCheckboxes map[int]widgets.Checkbox

	// emptyLabel is the "No messages." placeholder navigatePage creates
	// directly (not tracked in rowWidgets, since it isn't a
	// widgets.Container) when a page has zero rows -- 0 means none is
	// currently up. navigatePage destroys and clears this on every
	// rebuild before deciding whether the new page needs one, so
	// repeatedly landing on an empty page (e.g. Refresh clicked more than
	// once) never leaks more than one.
	emptyLabel widgets.Label
}

// folderUISnapshot mirrors folderUI's own widget-id fields only (not
// selection state -- see persistedFolderUIWidgets' own doc comment for
// why) -- folderUI's fields are unexported, so it can't be persisted
// directly (encoding/json silently drops unexported fields with no
// error).
//
// Real, load-bearing correction (found live): RowWidgets/RowCheckboxes
// are included here, NOT just the toolbar/container ids -- an earlier
// version of this fix left them out, reasoning they were pure selection
// bookkeeping that's fine to lose across a recycle. That was wrong:
// navigatePage's own row-rebuild (the common case once rowsContainer is
// already known, see its own `fu.rowsContainer == 0` fallback check)
// destroys the *previous* page's rows by iterating fu.rowWidgets, not by
// asking the host what rowsContainer's current children are -- with an
// empty, unrestored map, that destroy loop silently no-ops, and newly
// built rows just accumulate as extra siblings under the same
// rowsContainer instead of replacing anything (confirmed live: paging
// forward twice left both old pages' rows visibly stacked together).
// SelectedSeqs itself is still excluded -- losing an in-progress
// selection across a recycle is a real, accepted (non-destructive) gap,
// unlike losing the destroy-registry.
// RowWidgets/RowCheckboxes are string-keyed (the real seq, via
// strconv.Itoa -- see main.go's checkpoint/resume conversion), not
// map[int]... like folderUI's own live fields -- persistjson (the SDK's
// safe-marshal wrapper every Persisted[T] value goes through) only
// supports string-keyed maps, confirmed live ("unsupported value: map key
// type int").
type folderUISnapshot struct {
	RowsContainer widgets.Container            `json:"rows_container"`
	OlderBtn      widgets.Button               `json:"older_btn"`
	NewerBtn      widgets.Button               `json:"newer_btn"`
	PageLabel     widgets.Label                `json:"page_label"`
	DeleteBtn     widgets.Button               `json:"delete_btn"`
	EmptyLabel    widgets.Label                `json:"empty_label"`
	RowWidgets    map[string]widgets.Container `json:"row_widgets"`
	RowCheckboxes map[string]widgets.Checkbox  `json:"row_checkboxes"`
}

// persistedFolderUIWidgets persists the widget-id half of each folder's
// folderUI (2026-09-10, resume-without-recreate plan, Mechanism 1) --
// see folderUISnapshot's own doc comment for exactly what's included and
// why. Without the toolbar widget ids specifically, updateDeleteSelectedLabel
// silently can't update the Delete Selected button's own label at all (it
// no-ops on deleteBtn == 0) until an unrelated real rebuild happens to
// populate it as a side effect -- not a crash, but a real, avoidable gap
// this closes. Without RowWidgets/RowCheckboxes, navigatePage's own row
// destroy step silently no-ops -- see this type's own doc comment for the
// real, live-found bug this caused.
var persistedFolderUIWidgets = natyv.Persisted[map[string]folderUISnapshot]("folderUIWidgets", map[string]folderUISnapshot{})

// folderUIs persists each folder's own folderUI across folder switches --
// same lifetime as folderViews (deleted together by
// destroyPersistedFolderView), so a folder's selection/widget-handle state
// survives a plain "switch away and back" (folderViews' own SetVisible
// reuse) but not a real rebuild.
var folderUIs = map[string]*folderUI{}

// active is whichever folder's folderUI is currently on screen -- nil
// only before the very first folder is ever shown. Every row-selection
// helper below reads/writes through this instead of a bare package-level
// map, so it always acts on the folder actually visible right now, not
// whichever folder happened to build (or rebuild) most recently.
var active *folderUI

// activateFolder makes folder's own folderUI (creating one the first
// time) the active one -- called on every transition that puts a folder
// view on screen, whether a real build or a persisted-view reuse, so
// `active` never lags behind what's actually visible.
func activateFolder(folder string) {
	fu, ok := folderUIs[folder]
	if !ok {
		fu = &folderUI{
			selectedSeqs:  map[int]bool{},
			rowWidgets:    map[int]widgets.Container{},
			rowCheckboxes: map[int]widgets.Checkbox{},
		}
		folderUIs[folder] = fu
	}
	active = fu
}

// registerRowWidget/registerRowCheckbox are called once per real
// MessageRow build (views.go.ntx), right after creating each -- lets this
// file act on a specific row later (checkbox toggle bookkeeping, direct
// destroy after a batch delete or a page navigation) without MessageRow's
// own signature needing to return anything beyond its already-real
// `error`. Always registers into whichever folder is currently active --
// safe since a MessageRow is only ever built while its own folder is the
// one being built/rebuilt.
//
// Part 2 (codegen automation) regen, 2026-09-11: pure bookkeeping now --
// each tag's own bindKind="row_open"/"row_checkbox_toggle" bindArgs={...}
// (views.go.ntx) emits the real RegisterBinding call directly at creation
// time, so these two functions no longer need to duplicate it. rowArgs/
// rebindRowOpen/rebindRowCheckboxToggle below are unchanged -- still
// hand-written, since the real handlers are closures the safe-auto
// classifier correctly never attempts to resplice.
func registerRowWidget(seq int, w widgets.Container) {
    active.rowWidgets[seq] = w
}
func registerRowCheckbox(seq int, cb widgets.Checkbox) {
    active.rowCheckboxes[seq] = cb
}

// rowArgs is the persisted-args shape for both row_open and
// row_checkbox_toggle bindings. Folder is carried explicitly and
// validated at rebind-call time (see rebindRowOpen/
// rebindRowCheckboxToggle below), not just recorded -- Seq is only
// unique within one mailbox, and the implicit "hidden folder views are
// unclickable" safety net the real, ordinary build path relies on was
// never written to survive this reattachment path. A stale binding
// firing against the wrong now-current folder would mean acting on the
// wrong message (e.g. deleting the wrong email), not a cosmetic glitch.
type rowArgs struct {
    Seq    int    `json:"seq"`
    Folder string `json:"folder"`
}

func rebindRowOpen(widgetID uint32, args json.RawMessage) error {
    var a rowArgs
    if err := json.Unmarshal(args, &a); err != nil {
        return err
    }
    widgets.WrapContainer(widgetID).OnClick(func() error {
        if a.Folder != currentFolder {
            return nil
        }
        return showMessage(a.Seq)
    })
    return nil
}

func rebindRowCheckboxToggle(widgetID uint32, args json.RawMessage) error {
    var a rowArgs
    if err := json.Unmarshal(args, &a); err != nil {
        return err
    }
    checkbox := widgets.WrapCheckbox(widgetID)
    checkbox.OnClick(func() error {
        if a.Folder != currentFolder {
            return nil
        }
        checked, err := checkbox.Checked()
        if err != nil {
            return err
        }
        return onRowSelect(a.Seq, checked)
    })
    return nil
}

func init() {
    natyv.RegisterHandlerFunc("row_open", rebindRowOpen)
    natyv.RegisterHandlerFunc("row_checkbox_toggle", rebindRowCheckboxToggle)
}

// onRowSelect is MessageRow's own Checkbox onClick handler.
func onRowSelect(seq int, checked bool) error {
	if checked {
		active.selectedSeqs[seq] = true
	} else {
		delete(active.selectedSeqs, seq)
	}
	updateDeleteSelectedLabel()
	return nil
}

func updateDeleteSelectedLabel() {
	if active == nil || active.deleteBtn == 0 {
		return
	}
	_ = active.deleteBtn.SetLabel(fmt.Sprintf("Delete Selected (%d)", len(active.selectedSeqs)))
}

// resetSelectionState clears whatever's currently selected on the active
// folder. uncheckWidgets is true for the "hidden but still alive" case
// (clearView's own "folder:" branch) -- the row widgets/checkboxes
// themselves are untouched (still real, still alive, just no longer
// visible), only the *selection* is cleared and reflected back onto the
// still-real checkboxes. false means a real rebuild (or navigatePage's
// own row rebuild) is about to destroy every one of these widgets for
// real, so the id maps themselves are cleared too -- keeping them around
// would just be stale ids pointing at now-destroyed widgets.
func resetSelectionState(uncheckWidgets bool) {
	if active == nil {
		return
	}
	if uncheckWidgets {
		for _, cb := range active.rowCheckboxes {
			_ = cb.SetChecked(false)
		}
	} else {
		active.rowWidgets = map[int]widgets.Container{}
		active.rowCheckboxes = map[int]widgets.Checkbox{}
	}
	active.selectedSeqs = map[int]bool{}
	updateDeleteSelectedLabel()
}

// onDeleteSelected is the toolbar's own single "Delete Selected" button
// handler -- deletes every currently-checked row in one batch. Processes
// highest Seq first: deleting a message shifts every later message's real
// sequence number down by one (EXPUNGE's own behavior), so deleting
// top-down keeps every still-pending lower Seq valid throughout the batch
// instead of racing its own earlier deletions.
//
// Real UX fix (2026-09-03): this used to patch the current page's own
// cached message list in place (drop the deleted entries, shift remaining
// Seqs down) and destroy just the deleted rows' own widgets -- correct for
// what it did, but it never backfilled the now-short page with whatever
// used to be the *next* page's own leading message(s), leaving a
// permanently shorter page even when older mail exists to fill it. Fixed
// by dropping this page's own cache entry too (matching Refresh's own
// mechanism exactly, `renderFolder`'s Refresh closure just above) and
// calling `navigatePage` to re-fetch fresh from IMAP -- deriving the
// page's own [from, to] range against the real, now-decremented total
// message count naturally pulls in the right messages, and reuses the
// same already-proven rows-only rebuild every other pager action already
// goes through (no new "flash the whole view" risk).
func onDeleteSelected() error {
	seqs := make([]int, 0, len(active.selectedSeqs))
	for seq := range active.selectedSeqs {
		seqs = append(seqs, seq)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(seqs)))

	folder := currentFolder
	page := folderPage[folder]
	for _, seq := range seqs {
		if err := deleteMessage(seq); err != nil {
			return showError(err)
		}
	}

	delete(folderCache, folderCacheKey{folder, page})
	invalidateOtherPages(folder, page)

	return navigatePage(folder, page)
}

// pagerArgs is the persisted-args shape for the pager_nav binding (Older
// and Newer both share it, distinguished by Delta -- Older is +1, Newer
// is -1, confirmed against the real closures FolderView's own onOlder/
// onNewer params are built from). Folder, same reasoning as rowArgs
// above, is validated at rebind-call time, not just recorded.
type pagerArgs struct {
	Folder string `json:"folder"`
	Delta  int    `json:"delta"`
}

func rebindPagerNav(widgetID uint32, args json.RawMessage) error {
	var a pagerArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return err
	}
	widgets.WrapButton(widgetID).OnClick(func() error {
		if a.Folder != currentFolder {
			return nil
		}
		return navigatePage(a.Folder, folderPage[a.Folder]+a.Delta)
	})
	return nil
}

// rebindDeleteSelectedClick takes no args -- onDeleteSelected itself
// captures nothing, reading active/currentFolder/folderPage as live
// globals, same as the real, ordinary build path already does.
func rebindDeleteSelectedClick(widgetID uint32, args json.RawMessage) error {
	widgets.WrapButton(widgetID).OnClick(onDeleteSelected)
	return nil
}

// refreshArgs is refresh_click's own persisted-args shape -- Refresh's
// real closure (renderFolder's call site) closes over `folder string`
// only, same as pagerArgs above but without a Delta.
type refreshArgs struct {
	Folder string `json:"folder"`
}

func rebindRefreshClick(widgetID uint32, args json.RawMessage) error {
	var a refreshArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return err
	}
	widgets.WrapButton(widgetID).OnClick(func() error {
		if a.Folder != currentFolder {
			return nil
		}
		delete(folderCache, folderCacheKey{a.Folder, folderPage[a.Folder]})
		return navigatePage(a.Folder, folderPage[a.Folder])
	})
	return nil
}

// Part 2 (codegen automation) regen, 2026-09-11: rebindNavInbox/
// rebindNavSent/rebindNavCompose/rebindComposeSendClick are gone --
// handleShowInbox/handleShowSent/handleShowCompose/handleComposeSend are
// all already plain named functions with zero captured state (App/
// ComposeView take no relevant params), so codegen's own safe-auto
// classifier (isSafeToResplice + bare-identifier-only) now auto-generates
// both the RegisterBinding call and the rebind function for all four --
// nothing hand-written needed here anymore.

// rebindBackClick takes no args -- the real onBack closure
// (MessageDetailView's own call site) captures nothing, reading
// currentFolder live. Still hand-written: onBack is a composer param
// (MessageDetailView's own signature), so safe-auto correctly never
// attempts to resplice it -- bindKind="back_click" (views.go.ntx) hands
// off to this instead.
func rebindBackClick(widgetID uint32, args json.RawMessage) error {
	widgets.WrapButton(widgetID).OnClick(func() error { return showFolder(currentFolder) })
	return nil
}

// rebindPageSizeChange takes no args -- onPageSize captures nothing,
// reading pageSizeValues as a live global. widgets.WrapDropdown needs the
// same options list CreateDropdown (called inside FolderView's own
// generated code) was originally given -- pageSizeOptions itself, not a
// copy, since it's a fixed package-level var never mutated after init.
// Still hand-written: onPageSize is a composer param (FolderView's own
// signature), so safe-auto never attempts it -- bindKind="page_size_change"
// (views.go.ntx) hands off to this instead.
func rebindPageSizeChange(widgetID uint32, args json.RawMessage) error {
	widgets.WrapDropdown(widgetID, pageSizeOptions).OnSelect(onPageSize)
	return nil
}

func init() {
	natyv.RegisterHandlerFunc("pager_nav", rebindPagerNav)
	natyv.RegisterHandlerFunc("delete_selected_click", rebindDeleteSelectedClick)
	natyv.RegisterHandlerFunc("refresh_click", rebindRefreshClick)
	natyv.RegisterHandlerFunc("page_size_change", rebindPageSizeChange)
	natyv.RegisterHandlerFunc("back_click", rebindBackClick)
}

// currentView identifies whatever's actually rendered under viewRoot right
// now (e.g. "folder:INBOX", "message", "compose", "error") -- lets
// showFolder tell "already showing this" apart from "showing something
// else, or nothing yet", and lets clearView tell a persisted folder view
// (hide, don't destroy) apart from every other view (destroy as before).
// Every view-showing function below sets this itself, right alongside its
// own clearView() call, so it's never left stale by a transition this
// file doesn't know about. Also what rebuildApp (below) switches on to
// decide which content to reconstruct after a recycle -- persisted via
// persistedCurrentView so that decision survives one.
var currentView string
var persistedCurrentView = natyv.Persisted[string]("currentView", "")

// currentMessageSeq is whichever message's Seq is currently open in the
// read view -- only meaningful while currentView == "message". Set by
// showMessage alongside currentView itself; read by rebuildApp to
// re-fetch and re-show the same message after a recycle (readMessageBody
// re-selects currentFolder and re-fetches for real -- the same
// reconnect-and-redo pattern the phase2 spike already established for its
// own TCP connection, not a new idea). Persisted via
// persistedCurrentMessageSeq.
var currentMessageSeq int
var persistedCurrentMessageSeq = natyv.Persisted[int]("currentMessageSeq", 0)

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

// rebuildApp is this app's one whole-app region, rooted at appRoot: it
// recreates the toolbar + (empty) contentArea via App(parent), makes sure
// there's a live IMAP connection (connectIMAP is otherwise only ever
// called once, from natyv_init, and a resumed instance's own imapClient
// is nil -- the host force-closes every TCP connection on a recycle, the
// same contract the phase2 spike's own TCP connection already has), then
// re-renders whichever content currentView says was actually on screen.
// Called once from natyv_init (first boot, where currentView is still its
// Go zero value "") and once per resume, via the rebuild closure
// registered in init() below (natyv_resume's own hand-written code, see
// main.go, has already restored currentView/currentFolder/
// currentMessageSeq/pageSize/folderPage/appRoot from their Persisted[T]
// handles by the time this runs).
//
// Deliberately does NOT itself call SetActiveRegion -- whichever content
// function it delegates to (renderFolder/showMessage/showCompose) already
// does that at its own end, the same call every *normal* in-app
// navigation through that function also makes, so there's exactly one
// place per content kind that keeps the region's own recipe current,
// not two.
func rebuildApp(parent widgets.Container) error {
	if err := App(parent); err != nil {
		return err
	}
	// Part 2 (codegen automation) regen, 2026-09-11: App's own three
	// toolbar buttons no longer need registering here -- safe-auto now
	// emits their RegisterBinding call (and a real rebind function)
	// directly inside app.go.ntx's own generated output, since
	// handleShowInbox/handleShowSent/handleShowCompose are all bare,
	// resplice-safe identifiers.
	if err := connectIMAP(); err != nil {
		return err
	}
	switch {
	case strings.HasPrefix(currentView, "folder:"):
		return renderFolder(currentFolder, folderPage[currentFolder], true)
	case currentView == "message":
		// headerFor reads currentFolder's cached page to show the real
		// From/Subject alongside the body -- folderCache is deliberately
		// never persisted (it's a pure IMAP cache, safe to just refill on
		// demand), so warm it first or headerFor would come back empty
		// right after a resume even though the body itself fetches fine.
		if _, err := cachedListFolder(currentFolder, folderPage[currentFolder]); err != nil {
			return showError(err)
		}
		return showMessage(currentMessageSeq)
	case currentView == "compose":
		return showCompose()
	default:
		return handleShowInbox()
	}
}

func init() {
	natyv.RegisterRebuildFunc("rebuildApp", func(parent widgets.Container, args json.RawMessage) error {
		return rebuildApp(parent)
	})
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
	if fu, ok := folderUIs[folder]; ok && active == fu {
		active = nil
	}
	delete(folderUIs, folder)
	if viewRoot != nil && *viewRoot == root {
		viewRoot = nil
	}
}

func renderFolder(folder string, page int, forceRebuild bool) error {
	if root, ok := folderViews[folder]; ok && !forceRebuild && folderViewPage[folder] == page {
		// Already built, still fresh, and showing this exact page --
		// just swap it back in. No widgets were rebuilt, but currentView/
		// currentFolder DID change (e.g. switching back from Sent) --
		// still update the region's own recipe, or a recycle landing
		// right after this switch would incorrectly resume whatever was
		// showing *before* it instead of this folder.
		clearView()
		activateFolder(folder)
		currentFolder = folder
		currentView = "folder:" + folder
		viewRoot = &root
		if err := root.SetVisible(true); err != nil {
			return err
		}
		// SetActiveRegion is no longer called here (2026-09-10,
	// resume-without-recreate plan, step 7 cleanup) -- nothing ever reads
	// regionRegistry's stored recipes anymore now that natyv_checkpoint
	// calls SnapshotBindings instead of SnapshotRegions, so this would
	// just be building up dead state on every real navigation.
	return nil
	}
	destroyPersistedFolderView(folder)
	clearView()
	activateFolder(folder)

	currentFolder = folder
	currentView = "folder:" + folder
	folderPage[folder] = page
	data, err := cachedListFolder(folder, page)
	if err != nil {
		return showError(err)
	}
	// Part 2 (codegen automation) regen, 2026-09-11: folder is now a real
	// FolderView param (views.go.ntx), specifically so refresh_click/
	// pager_nav's own bindKind=/bindArgs= have it in scope -- closes the
	// one residual case the plan's own §1(c) documented (folder previously
	// existed only in this closure's own scope, never passed into
	// FolderView at all).
	if err := FolderView(*contentArea, folder, toInboxRows(data.msgs), folderDisplayName(folder), page, fmt.Sprintf("Page %d", page+1), data.canGoOlder, pageSizeLabel(),
		func() error { return navigatePage(folder, folderPage[folder]-1) },
		func() error { return navigatePage(folder, folderPage[folder]+1) },
		func() error {
			// Manual refresh -- Inbox's own cache is never auto-invalidated
			// by a send (unlike Sent, which we know for certain gets a new
			// entry), so this is the way to actually see new mail without
			// restarting the app. Goes through navigatePage too, same as
			// Older/Newer, so Refresh no longer flashes the whole view --
			// just the rows.
			delete(folderCache, folderCacheKey{folder, folderPage[folder]})
			return navigatePage(folder, folderPage[folder])
		},
		onPageSize,
		onDeleteSelected,
		func() error {
			// Pure harvesting now -- every RegisterBinding call that used
			// to live here (pager_nav x2, delete_selected_click,
			// refresh_click, page_size_change) is now emitted directly at
			// each tag's own creation site in views.go.ntx instead
			// (bindKind=/bindArgs=, or automatically for the two that
			// don't need any -- none of these five actually qualify for
			// safe-auto, since onOlder/onNewer/onDeleteSelected/onPageSize
			// are all composer params).
			fu := folderUIs[folder]
			fu.rowsContainer = *rowsContainer
			fu.olderBtn = *olderBtn
			fu.newerBtn = *newerBtn
			fu.pageLabel = *pagerLabelWidget
			fu.deleteBtn = *deleteSelectedBtn
			return nil
		},
	); err != nil {
		return err
	}
	folderViews[folder] = *viewRoot
	folderViewPage[folder] = page
	updateDeleteSelectedLabel()
	// SetActiveRegion is no longer called here (2026-09-10,
	// resume-without-recreate plan, step 7 cleanup) -- nothing ever reads
	// regionRegistry's stored recipes anymore now that natyv_checkpoint
	// calls SnapshotBindings instead of SnapshotRegions, so this would
	// just be building up dead state on every real navigation.
	return nil
}

// navigatePage swaps folder's currently-displayed page to newPage while
// leaving the rest of its view (label, toolbar, pager buttons) alive --
// only the message rows themselves are destroyed and rebuilt, avoiding
// the old full-view rebuild flash on every Older/Newer/Refresh click.
// Falls back to a real renderFolder if folder has no live view yet to
// update in place (shouldn't happen in practice -- every caller only
// ever reaches this through a button that's only clickable while its own
// folder's view is already built and on screen -- but a real fallback
// costs nothing and avoids a nil-pointer panic if that assumption is ever
// wrong).
func navigatePage(folder string, newPage int) error {
	fu, ok := folderUIs[folder]
	if !ok || fu.rowsContainer == 0 {
		return renderFolder(folder, newPage, false)
	}
	active = fu

	data, err := cachedListFolder(folder, newPage)
	if err != nil {
		return showError(err)
	}

	for _, w := range fu.rowWidgets {
		w.Destroy()
	}
	fu.rowWidgets = map[int]widgets.Container{}
	fu.rowCheckboxes = map[int]widgets.Checkbox{}
	fu.selectedSeqs = map[int]bool{}
	if fu.emptyLabel != 0 {
		fu.emptyLabel.Destroy()
		fu.emptyLabel = 0
	}

	rows := toInboxRows(data.msgs)
	if len(rows) == 0 {
		// Matches the bare <Label text={"No messages."} /> FolderView's
		// own markup builds for this same case -- same hardcoded
		// Fixed(300)/Fixed(24) a styleless <Label> tag gets from .ntx's
		// own LayoutDefaults, so this path looks identical to a real
		// FolderView build.
		pid := uint32(fu.rowsContainer)
		label, err := widgets.CreateLabel(widgets.Layout{
			ParentID: &pid,
			Sizing:   widgets.Sizing{Width: widgets.Fixed(300), Height: widgets.Fixed(24)},
		}, "No messages.")
		if err != nil {
			return err
		}
		fu.emptyLabel = label
	} else {
		// Newest first, matching FolderView's own loop.
		for i := len(rows) - 1; i >= 0; i-- {
			row := rows[i]
			seq := row.Seq
			if err := MessageRow(uint32(fu.rowsContainer), seq, row.From, row.Subject,
				func() error { return showMessage(seq) }, onRowSelect); err != nil {
				return err
			}
		}
	}

	folderPage[folder] = newPage
	folderViewPage[folder] = newPage

	if err := fu.pageLabel.SetText(fmt.Sprintf("Page %d", newPage+1)); err != nil {
		return err
	}
	if err := fu.olderBtn.SetEnabled(data.canGoOlder); err != nil {
		return err
	}
	if err := fu.newerBtn.SetEnabled(newPage > 0); err != nil {
		return err
	}
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
	currentMessageSeq = seq
	if err := MessageDetailView(*contentArea, truncateWithEllipsis(from, 100), truncateWithEllipsis(subject, 100), body, func() error { return showFolder(currentFolder) }); err != nil {
		return err
	}
	// Part 2 (codegen automation) regen, 2026-09-11: no longer registered
	// here -- bindKind="back_click" on the Back button's own tag
	// (views.go.ntx) emits the RegisterBinding call directly at creation.
	// SetActiveRegion is no longer called here (2026-09-10,
	// resume-without-recreate plan, step 7 cleanup) -- nothing ever reads
	// regionRegistry's stored recipes anymore now that natyv_checkpoint
	// calls SnapshotBindings instead of SnapshotRegions, so this would
	// just be building up dead state on every real navigation.
	return nil
}

// -- Compose view --

func showCompose() error {
	clearView()
	currentView = "compose"
	if err := ComposeView(*contentArea); err != nil {
		return err
	}
	// Part 2 (codegen automation) regen, 2026-09-11: no longer registered
	// here -- handleComposeSend is a bare, resplice-safe identifier
	// (ComposeView takes no params), so safe-auto now emits both the
	// RegisterBinding call and a real rebind function directly.
	// SetActiveRegion is no longer called here (2026-09-10,
	// resume-without-recreate plan, step 7 cleanup) -- nothing ever reads
	// regionRegistry's stored recipes anymore now that natyv_checkpoint
	// calls SnapshotBindings instead of SnapshotRegions, so this would
	// just be building up dead state on every real navigation.
	return nil
}

// -- Toolbar handlers, referenced by app.go.ntx's onClick={...} bindings --

func handleShowInbox() error   { return showFolder(inboxFolder) }
func handleShowSent() error    { return showFolder(sentFolder) }
func handleShowCompose() error { return showCompose() }
