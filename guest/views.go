package main

import "github.com/natyv-io/sdks/go/widgets"







func ErrorView(parent widgets.Container, message string) error {
	return natyvBuildErrorView(parent, message)
}

func FolderView(parent widgets.Container, rows []inboxRow, folderLabel string, page int, pageLabel string, canGoOlder bool, pageSizeLabel string, onNewer func() error, onOlder func() error, onRefresh func() error, onPageSize func(index int) error, onDeleteSelected func() error) error {
	return natyvBuildFolderView(parent, rows, folderLabel, page, pageLabel, canGoOlder, pageSizeLabel, onNewer, onOlder, onRefresh, onPageSize, onDeleteSelected)
}

// MessageRow's own top-level <Container> is a purely structural wrapper --
// <%...%> can't be a composer's own top-level node (Parser.zig only ever
// parses exactly one top-level element), so declaring rowWidget/checkbox
// as real local variables -- needed so ref={&rowWidget}/ref={&checkbox}
// bind correctly per invocation, not to a single shared package-level
// slot the way every other ref={&x} in this app does, since FolderView's
// own loop creates many MessageRows alive at once -- has to happen inside
// a <%...%> child, one level down. The row itself is directly clickable
// (onClick={onOpen} on rowWidget, a real Container -- 2026-09-02) rather
// than a separate "Open" button; a real natyv-core fix made this possible
// and correctly suppresses the row's own click when the nested Checkbox
// is what's actually clicked.
func MessageRow(parent uint32, seq int, from string, subject string, onOpen func() error, onSelect func(seq int, checked bool) error) error {
	return natyvBuildMessageRow(parent, seq, from, subject, onOpen, onSelect)
}

func MessageDetailView(parent widgets.Container, from string, subject string, body string, onBack func() error) error {
	return natyvBuildMessageDetailView(parent, from, subject, body, onBack)
}

func ComposeView(parent widgets.Container) error {
	return natyvBuildComposeView(parent)
}
