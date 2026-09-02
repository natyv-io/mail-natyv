package main

import "github.com/natyv-io/sdks/go/widgets"







func ErrorView(parent widgets.Container, message string) error {
	return natyvBuildErrorView(parent, message)
}

func FolderView(parent widgets.Container, rows []inboxRow, page int, canGoOlder bool, pageSizeLabel string, onNewer func() error, onOlder func() error, onRefresh func() error, onPageSize func(index int) error) error {
	return natyvBuildFolderView(parent, rows, page, canGoOlder, pageSizeLabel, onNewer, onOlder, onRefresh, onPageSize)
}

// MessageRow's own top-level <Container> is a purely structural wrapper --
// <%...%> can't be a composer's own top-level node (Parser.zig only ever
// parses exactly one top-level element), so declaring rowWidget as a real
// local variable -- needed so ref={&rowWidget} binds correctly per
// invocation, not to a single shared package-level slot the way every
// other ref={&x} in this app does, since FolderView's own loop creates
// many MessageRows alive at once -- has to happen inside a <%...%> child,
// one level down. Real, disclosed cost: today's styling system has no
// NTSS field to zero out a bare <Container>'s own hardcoded default
// padding (8px on every side), so this wrapper adds a small, visible 8px
// inset around every row that the original hand-written version didn't
// have -- a real gap in the current styling feature set, not swept under
// the rug.
func MessageRow(parent uint32, from string, subject string, onOpen func() error, onDelete func() error) error {
	return natyvBuildMessageRow(parent, from, subject, onOpen, onDelete)
}

func MessageDetailView(parent widgets.Container, from string, subject string, body string, onBack func() error) error {
	return natyvBuildMessageDetailView(parent, from, subject, body, onBack)
}

func ComposeView(parent widgets.Container) error {
	return natyvBuildComposeView(parent)
}
