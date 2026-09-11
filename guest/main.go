package main

import (
    "strconv"

    "github.com/natyv-io/sdks/go/widgets"

    natyv "github.com/natyv-io/sdks/go"
    "github.com/extism/go-pdk"
)

//go:wasmexport natyv_init
func natyvInit() int32 {
    if err := natyv.FlushPersisted(); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    r, err := widgets.CreateContainer(appRootLayout, false, 0)
    if err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    appRoot = r
    if err := persistedAppRoot.Set(r); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }

    if err := rebuildApp(appRoot); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    return 0
}

// Part 2 (codegen automation) regen, 2026-09-11: contentArea/viewRoot/
// toField/subjField/bodyField/statusLbl are no longer hand-persisted
// here -- every one of them is a plain ref='d, non-struct-backed widget,
// so GeneratedCheckpoint (below, via genSnapshotRefs) now handles all six
// automatically. What's left here is exactly the plan's own §3.7 "honest
// limit": real app data with no .ntx connection at all (currentFolder/
// pageSize/folderPage/currentView/currentMessageSeq/folderViews/
// folderViewPage), plus folderUIs' own dynamic, string-keyed
// widgetSnap -- genuinely invisible to codegen by construction, not by
// omission.
//
//go:wasmexport natyv_checkpoint
func natyvCheckpoint() int32 {
    if err := persistedCurrentFolder.Set(currentFolder); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if err := persistedPageSize.Set(pageSize); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if err := persistedFolderPage.Set(folderPage); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if err := persistedCurrentView.Set(currentView); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if err := persistedCurrentMessageSeq.Set(currentMessageSeq); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if err := persistedFolderViews.Set(folderViews); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if err := persistedFolderViewPage.Set(folderViewPage); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    widgetSnap := make(map[string]folderUISnapshot, len(folderUIs))
    for folder, fu := range folderUIs {
        rowWidgets := make(map[string]widgets.Container, len(fu.rowWidgets))
        for seq, w := range fu.rowWidgets {
            rowWidgets[strconv.Itoa(seq)] = w
        }
        rowCheckboxes := make(map[string]widgets.Checkbox, len(fu.rowCheckboxes))
        for seq, cb := range fu.rowCheckboxes {
            rowCheckboxes[strconv.Itoa(seq)] = cb
        }
        widgetSnap[folder] = folderUISnapshot{
            RowsContainer: fu.rowsContainer,
            OlderBtn:      fu.olderBtn,
            NewerBtn:      fu.newerBtn,
            PageLabel:     fu.pageLabel,
            DeleteBtn:     fu.deleteBtn,
            EmptyLabel:    fu.emptyLabel,
            RowWidgets:    rowWidgets,
            RowCheckboxes: rowCheckboxes,
        }
    }
    if err := persistedFolderUIWidgets.Set(widgetSnap); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    data, err := GeneratedCheckpoint()
    if err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    pdk.Output(data)
    return 0
}

//go:wasmexport natyv_resume
func natyvResume() int32 {
    if err := natyv.FlushPersisted(); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    var err error
    if appRoot, err = persistedAppRoot.Get(); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if currentFolder, err = persistedCurrentFolder.Get(); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if pageSize, err = persistedPageSize.Get(); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if folderPage, err = persistedFolderPage.Get(); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if currentView, err = persistedCurrentView.Get(); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if currentMessageSeq, err = persistedCurrentMessageSeq.Get(); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    // Real mechanism (~/.claude/plans/adaptive-painting-seal.md), replacing
    // RestoreRegions' mass destroy+recreate entirely: reattach handlers to
    // the widgets that are already sitting there host-side, untouched by
    // recycling regardless. connectIMAP is real and required here now
    // (previously only ever called from inside rebuildApp, which resume no
    // longer calls) -- without it imapClient stays nil and the first real
    // IMAP call from a reattached handler nil-panics (found live via Step
    // 0's own testing).
    if err := connectIMAP(); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    // Part 2 (codegen automation) regen, 2026-09-11: contentArea/viewRoot/
    // toField/subjField/bodyField/statusLbl restoration is no longer
    // hand-written here -- GeneratedResume (below, via genRestoreRefs)
    // handles all six automatically now.
    // TEMPORARY -- step 13 live-testing aid only, remove after testing.
    if growMemoryClickCount, err = persistedGrowMemoryClickCount.Get(); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if folderViews, err = persistedFolderViews.Get(); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if folderViewPage, err = persistedFolderViewPage.Get(); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    // activateFolder populates `active` (a real, empty *folderUI, not nil)
    // for currentFolder -- without this, any reattached handler that reads
    // `active` (onRowSelect, onDeleteSelected, updateDeleteSelectedLabel)
    // nil-panics the moment it's clicked, before any real rebuild has had
    // a chance to run activateFolder itself as a side effect (found live:
    // a checkbox click and a Delete Selected click both crashed this way
    // on a fresh resumed instance's very first interaction). rowWidgets/
    // rowCheckboxes are restored below (from widgetSnap, not by this call
    // alone) -- without them, navigatePage's own destroy-before-rebuild
    // step silently no-ops (found live: paged rows accumulated instead of
    // replacing each other). Real, known gap this still doesn't close:
    // selectedSeqs itself isn't persisted, so an in-progress selection
    // doesn't survive a recycle -- accepted, non-destructive.
    widgetSnap, err := persistedFolderUIWidgets.Get()
    if err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    for folder, snap := range widgetSnap {
        activateFolder(folder)
        fu := folderUIs[folder]
        fu.rowsContainer = snap.RowsContainer
        fu.olderBtn = snap.OlderBtn
        fu.newerBtn = snap.NewerBtn
        fu.pageLabel = snap.PageLabel
        fu.deleteBtn = snap.DeleteBtn
        fu.emptyLabel = snap.EmptyLabel
        if snap.RowWidgets != nil {
            rowWidgets := make(map[int]widgets.Container, len(snap.RowWidgets))
            for seqStr, w := range snap.RowWidgets {
                if seq, err := strconv.Atoi(seqStr); err == nil {
                    rowWidgets[seq] = w
                }
            }
            fu.rowWidgets = rowWidgets
        }
        if snap.RowCheckboxes != nil {
            rowCheckboxes := make(map[int]widgets.Checkbox, len(snap.RowCheckboxes))
            for seqStr, cb := range snap.RowCheckboxes {
                if seq, err := strconv.Atoi(seqStr); err == nil {
                    rowCheckboxes[seq] = cb
                }
            }
            fu.rowCheckboxes = rowCheckboxes
        }
    }
    activateFolder(currentFolder)
    if err := GeneratedResume(pdk.Input()); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }

    return 0
}

func main() {}
