package main

import (
	_ "embed"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// iconSmallPNG is a simplified bold rendition of the logo for small sizes
// (taskbar, tray, panel mark). Fine detail (pod ring, dots, leaf) does not
// survive 16-32px, so small sizes use three chunky windows with the first
// highlighted. The full-detail assets/icon.png feeds the exe resource and
// the README.
//
//go:embed assets/icon-small.png
var iconSmallPNG []byte

func main() {
	a := newApp()
	// The panel is native UI; only guest pages call Go, through Sync.
	mygo.BindAs("Sync", &SyncService{a})

	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:          "Peapod",
			Width:          1440,
			Height:         900,
			MinWidth:       1000,
			MinHeight:      640,
			StateKey:       "main2",
			TitleBarStyle:  mygo.TitleBarHidden,
			TitleBarHeight: titleBarHeight,
			Content:        ui.View(a.view),
		})
		if err := mygo.App.Dock.SetIcon(iconSmallPNG); err != nil {
			log.Printf("set app icon: %v", err)
		}
		if err := a.startMCP(); err != nil {
			log.Printf("start mcp: %v", err)
		}
		a.win.Invalidate()
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
