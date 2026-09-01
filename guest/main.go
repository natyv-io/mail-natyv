package main

import (
    "github.com/natyv-io/sdks/go/widgets"

    "github.com/extism/go-pdk"
)

//go:wasmexport natyv_init
func natyvInit() int32 {
    root, err := widgets.CreateContainer(widgets.Layout{
        Sizing: widgets.Sizing{Width: widgets.Grow(), Height: widgets.Grow()},
    }, false, 0)
    if err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if err := App(root); err != nil {
        pdk.SetErrorString(err.Error())
        return 1
    }
    if err := connectIMAP(); err != nil {
        pdk.SetErrorString("connect: " + err.Error())
        return 1
    }
    if err := handleShowInbox(); err != nil {
        pdk.SetErrorString("show inbox: " + err.Error())
        return 1
    }
    return 0
}

func main() {}
