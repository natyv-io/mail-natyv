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
// time it runs.
func FolderView(parent widgets.Container, rows []inboxRow, folderLabel string, page int, pageLabel string, canGoOlder bool, pageSizeLabel string, onNewer func() error, onOlder func() error, onRefresh func() error, onPageSize func(index int) error, onDeleteSelected func() error, onBuilt func() error) error {
	return natyvBuildFolderView(parent, rows, folderLabel, page, pageLabel, canGoOlder, pageSizeLabel, onNewer, onOlder, onRefresh, onPageSize, onDeleteSelected, onBuilt)
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
func MessageRow(parent uint32, seq int, from string, subject string, onOpen func() error, onSelect func(seq int, checked bool) error) error {
	return natyvBuildMessageRow(parent, seq, from, subject, onOpen, onSelect)
}

func MessageDetailView(parent widgets.Container, from string, subject string, body string, onBack func() error) error {
	return natyvBuildMessageDetailView(parent, from, subject, body, onBack)
}

func ComposeView(parent widgets.Container) error {
	return natyvBuildComposeView(parent)
}
