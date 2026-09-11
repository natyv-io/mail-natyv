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
// pageSize/folderPage/currentView/currentMessageUID/folderViews/
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
    if err := persistedCurrentMessageUID.Set(currentMessageUID); err != nil {
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
        for uid, w := range fu.rowWidgets {
            rowWidgets[strconv.Itoa(uid)] = w
        }
        rowCheckboxes := make(map[string]widgets.Checkbox, len(fu.rowCheckboxes))
        for uid, cb := range fu.rowCheckboxes {
            rowCheckboxes[strconv.Itoa(uid)] = cb
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
    // messageDetailView/composeViewContainer (garbled-text fix, 2026-09-11)
    // are hand-derived copies of *viewRoot, not themselves ref= targets --
    // Mechanism 1 has nothing to persist them with, same reasoning
    // folderViews above already established.
    if err := persistedMessageDetailView.Set(messageDetailView); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if err := persistedComposeViewContainer.Set(composeViewContainer); err != nil {
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
    if currentMessageUID, err = persistedCurrentMessageUID.Get(); err != nil {
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
    // Real bug, found 2026-09-11 (the same session as the garbled-text fix
    // above, but a genuinely separate cause): folderCache is deliberately
    // never persisted (it's a pure IMAP cache, safe to just refill on
    // demand) -- but this means it starts flat-out empty on every fresh
    // guest instance a recycle produces. FolderView's own rows survive a
    // recycle fine now (pooled/persisted widgets, untouched host-side), so
    // clicking one right after a recycle -- before anything else happens to
    // repopulate the cache -- reaches headerFor with nothing to find,
    // returning "", "" for a message that's really is sitting right there.
    // This exact risk was already anticipated once (see the near-identical
    // comment that used to live in rebuildApp's own "case currentView ==
    // message" branch), but that mitigation went dead the day resume
    // stopped calling rebuildApp at all (2026-09-10, resume-without-
    // recreate) -- nobody re-homed it to where resume actually lives now.
    // Fixed by warming exactly the one (folder, page) pair that matters:
    // whatever's cached here is what headerFor's own lookup key resolves
    // to the moment any currently-visible row gets clicked next, message
    // view or folder view alike.
    if _, err := cachedListFolder(currentFolder, folderPage[currentFolder]); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    // Part 2 (codegen automation) regen, 2026-09-11: contentArea/viewRoot/
    // toField/subjField/bodyField/statusLbl restoration is no longer
    // hand-written here -- GeneratedResume (below, via genRestoreRefs)
    // handles all six automatically now.
    if folderViews, err = persistedFolderViews.Get(); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if folderViewPage, err = persistedFolderViewPage.Get(); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    // messageDetailView/composeViewContainer (garbled-text fix, 2026-09-11)
    // -- restored before GeneratedResume runs, same as folderViews above,
    // since their own widgets survive a recycle regardless (host-side,
    // untouched) but the guest-side var naming them does not.
    if messageDetailView, err = persistedMessageDetailView.Get(); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if composeViewContainer, err = persistedComposeViewContainer.Get(); err != nil {
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
    // selectedUIDs itself isn't persisted, so an in-progress selection
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
            for uidStr, w := range snap.RowWidgets {
                if uid, err := strconv.Atoi(uidStr); err == nil {
                    rowWidgets[uid] = w
                }
            }
            fu.rowWidgets = rowWidgets
        }
        if snap.RowCheckboxes != nil {
            rowCheckboxes := make(map[int]widgets.Checkbox, len(snap.RowCheckboxes))
            for uidStr, cb := range snap.RowCheckboxes {
                if uid, err := strconv.Atoi(uidStr); err == nil {
                    rowCheckboxes[uid] = cb
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
