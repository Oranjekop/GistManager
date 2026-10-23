package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Oranjekop/GistManager/internal/app"
	"github.com/Oranjekop/GistManager/internal/gist"
	"github.com/Oranjekop/GistManager/internal/security"
	"github.com/Oranjekop/GistManager/internal/update"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

//go:embed resources/icon.png
var icon []byte

//go:embed resources/update-public.key
var publicKey string

//go:embed mygo.json
var configuration []byte

func main() {
	dataDir := flag.String("data-dir", "", "Override the user data directory")
	demo := flag.Bool("demo", false, "Use isolated, in-memory demo data")
	smoke := flag.String("smoke", "", "Capture the native window to a PNG, then quit")
	apply := flag.String("apply-update", "", "Apply a previously verified update")
	noRelaunch := flag.Bool("no-relaunch", false, "Do not relaunch after applying an update (verification)")
	versionOnly := flag.Bool("version", false, "Print the application version")
	flag.Parse()
	var config struct{ Name, Version string }
	if err := json.Unmarshal(configuration, &config); err != nil {
		log.Fatal(err)
	}
	if *versionOnly {
		fmt.Println(config.Version)
		return
	}
	if update.PublicKey == "" {
		update.PublicKey = strings.TrimSpace(publicKey)
	}
	if *apply != "" {
		if err := update.ApplyPlan(*apply, update.PublicKey, !*noRelaunch); err != nil {
			_ = os.WriteFile(filepath.Join(filepath.Dir(*apply), "update-error.log"), []byte(err.Error()), 0o600)
			log.Fatal(err)
		}
		return
	}
	mygo.App.SetName(config.Name)
	mygo.App.SetVersion(config.Version)
	if os.Getenv("MYGO_GENERATE") != "" {
		if err := mygo.App.Run(); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *dataDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			log.Fatal(err)
		}
		*dataDir = filepath.Join(base, config.Name)
		if mygo.IsDev() {
			*dataDir += "-dev"
		}
		if !*demo && !mygo.IsDev() {
			if err := security.ImportLegacy(*dataDir, base); err != nil {
				log.Fatal(err)
			}
		}
	}
	if *demo {
		var err error
		*dataDir, err = os.MkdirTemp("", "gist-manager-demo-*")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(*dataDir)
	}
	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		log.Fatal(err)
	}
	mygo.App.SetPath(mygo.PathUserData, *dataDir)
	if !*demo && !mygo.App.RequestSingleInstanceLock() {
		return
	}
	var vault security.Vault = security.SystemVault{}
	var biometric security.Biometric = security.SystemBiometric{}
	if *demo {
		vault, biometric = &app.DemoVault{}, app.DemoBiometric{}
	}
	secure, err := security.Open(*dataDir, vault, biometric)
	if err != nil {
		log.Fatal(err)
	}
	var api app.API = gist.NewClient()
	if *demo {
		api = app.NewDemoAPI()
	}
	a := app.New(api, secure, update.New(*dataDir, config.Version), config.Version, *dataDir)
	a.Icon, err = ui.DecodeBitmap(icon)
	if err != nil {
		log.Fatal(err)
	}
	a.LoadLayout()
	if *demo {
		a.LoadDemo()
	}
	if runtime.GOOS != "darwin" {
		mygo.App.SetMenu(nil)
	}
	mygo.App.WhenReady(func() {
		a.Window = mygo.NewWindow(mygo.WindowOptions{
			Title: config.Name, Width: 1500, Height: 940, MinWidth: 1000, MinHeight: 680,
			Content: ui.View(a.View),
		})
		// Older Windows update helpers pass SW_HIDE to the relaunched app.
		// Show the workspace before startup authentication can open its prompt.
		if !a.Window.IsVisible() {
			a.Window.Show()
		}
		if err := a.Window.SetIcon(icon); err != nil {
			log.Printf("icon: %v", err)
		}
		mygo.App.OnSecondInstance(func([]string, string) {
			if a.Window != nil && !a.Window.IsDestroyed() {
				a.Window.Show()
				a.Window.Focus()
			}
		})
		a.Window.OnClose(func(event *mygo.CloseEvent) {
			if !a.RequestClose() {
				event.PreventDefault()
			}
		})
		mygo.App.OnBeforeQuit(func(event *mygo.QuitEvent) {
			if !a.RequestClose() {
				event.PreventDefault()
			}
		})
		if !*demo {
			a.Start()
		}
		if *smoke != "" {
			time.AfterFunc(3*time.Second, func() {
				png, captureErr := a.Window.CapturePage()
				if captureErr == nil {
					captureErr = os.WriteFile(*smoke, png, 0o600)
				}
				if captureErr != nil {
					log.Printf("capture: %v", captureErr)
				}
				mygo.App.Quit()
			})
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
