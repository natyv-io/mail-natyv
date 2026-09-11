package main

import "github.com/natyv-io/sdks/go/widgets"







func ErrorView(parent widgets.Container, message string) error {
	return natyvBuildErrorView(parent, message)
}

// onBuilt fires once, at the very end of a real build, after every ref
// above it (rowsContainer/olderBtn/newerBtn/pagerLabelWidget/
// deleteSelectedBtn) has already been assigned -- lets ui.go harvest them
// into the just-activated folder's own persistent record (see
// activateFolder/folderUI) without FolderView itself needing to know
// anything about per-folder bookkeeping. Takes no params: everything it
// needs is already sitting in these same package-level ref slots by the
// time it runs. Part 2 (codegen automation) regen, 2026-09-11: onBuilt no
// longer also registers pager_nav/delete_selected_click/refresh_click/
// page_size_change bindings itself -- bindKind=/bindArgs= on each tag
// above does that automatically now (folder, promoted to a real param
// here specifically to make refresh_click/pager_nav's own bindArgs
// possible), so onBuilt is back to pure harvesting.
func FolderView(parent widgets.Container, folder string, rows []inboxRow, folderLabel string, page int, pageLabel string, canGoOlder bool, pageSizeLabel string, onNewer func() error, onOlder func() error, onRefresh func() error, onPageSize func(index int) error, onDeleteSelected func() error, onBuilt func() error) error {
	return natyvBuildFolderView(parent, folder, rows, folderLabel, page, pageLabel, canGoOlder, pageSizeLabel, onNewer, onOlder, onRefresh, onPageSize, onDeleteSelected, onBuilt)
}

// MessageRow's own top-level element is its real, directly-clickable row
// Container (onClick={onOpen}, a real Container -- 2026-09-02) -- no
// purely-structural outer wrapper anymore (2026-09-02): the previous
// wrapper existed only so ref={&rowWidget} could bind to a fresh
// per-invocation local rather than this app's usual shared package-level
// ref slot, but rowWidget never actually needed that -- nothing closes
// over it the way the Checkbox's own onClick closure genuinely needs a
// fresh `checkbox` local per row. Ref-binding rowWidget directly to a
// plain package-level slot (registerRowWidget captures its value
// immediately, before the next MessageRow call can overwrite it) works
// exactly like every other single-widget-at-a-time ref in this app, and
// means the id navigatePage destroys per row is genuinely this row's own
// entire subtree -- the old wrapper would have been silently orphaned
// (leaked) on every rebuild instead of destroyed, since only the inner
// ref'd Container was ever tracked. `checkbox` is still a real
// `<%...%>`-scoped local: FolderView's own loop keeps many MessageRows
// alive at once, each with its own permanently-registered Checkbox
// onClick closure, so it can't share a single package-level slot the way
// rowWidget now safely does.
func MessageRow(parent uint32, uid int, from string, subject string, onOpen func() error, onSelect func(uid int, checked bool) error) error {
	return natyvBuildMessageRow(parent, uid, from, subject, onOpen, onSelect)
}

// From/Subject/Body all carry their own ref= now (2026-09-11, garbled-text
// investigation) so showMessage can update them in place on a pooled,
// already-built view instead of destroying and recreating this whole
// subtree on every single message open -- see ui.go's own doc comment on
// messageDetailView for the real reasoning. body arrives already
// MIME-decoded (showMessage's own job now, not this composer's) so the
// same already-decoded string can be handed to SetText on the reuse path
// without decoding it twice.
func MessageDetailView(parent widgets.Container, from string, subject string, body string, onBack func() error) error {
	return natyvBuildMessageDetailView(parent, from, subject, body, onBack)
}

func ComposeView(parent widgets.Container) error {
	return natyvBuildComposeView(parent)
}
