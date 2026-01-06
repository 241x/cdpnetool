package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"cdpnetool/internal/browser"
	ilog "cdpnetool/internal/log"
	api "cdpnetool/pkg/api"
	"cdpnetool/pkg/model"
)

func main() {
	devtools := os.Getenv("DEVTOOLS_URL")

	l := log.New(os.Stdout, "", 0)

	var err error
	var br *browser.Browser
	if devtools == "" {
		br, err = browser.Start(browser.Options{Headless: false})
		if err != nil {
			fmt.Println("start browser error:", err)
			return
		}
		devtools = br.DevToolsURL
	}

	svc := api.NewServiceWithLogger(ilog.New(l))
	cfg := model.SessionConfig{
		DevToolsURL:       devtools,
		Concurrency:       4,
		BodySizeThreshold: 4 * 1024 * 1024,
		PendingCapacity:   64,
		ProcessTimeoutMS:  200,
	}
	id, err := svc.StartSession(cfg)
	if err != nil {
		fmt.Println("start session error:", err)
		return
	}
	defer svc.StopSession(id)

	if err = svc.AttachTarget(id, ""); err != nil {
		fmt.Println("attach target error:", err)
		return
	}
	if err = svc.EnableInterception(id); err != nil {
		fmt.Println("enable interception error:", err)
		return
	}

	rs := model.RuleSet{
		Version: "1.0",
		Rules: []model.Rule{
			{
				ID:       model.RuleID("demo_resp_patch"),
				Priority: 100,
				Mode:     "short_circuit",
				Match: model.Match{
					AllOf: []model.Condition{
						{Type: "mime", Mode: "prefix", Pattern: "application/json"},
					},
				},
				Action: model.Action{
					Rewrite: &model.Rewrite{
						Body: &model.BodyPatch{Type: "json_patch", Ops: []any{
							map[string]any{"op": "add", "path": "/_cdpnetool/demo", "value": true},
						}},
					},
				},
			},
		},
	}
	_ = svc.LoadRules(id, rs)

	evc, err := svc.SubscribeEvents(id)
	if err != nil {
		fmt.Println("subscribe events error:", err)
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	go func() {
		for e := range evc {
			if e.Rule != nil {
				fmt.Println("event:", e.Type, "rule:", *e.Rule)
			} else {
				fmt.Println("event:", e.Type)
			}
		}
	}()

	if br != nil {
		fmt.Println("demo running. browser launched at", devtools)
	} else {
		fmt.Println("demo running. ensure your browser is started and DEVTOOLS_URL set")
	}
	<-ctx.Done()
	time.Sleep(200 * time.Millisecond)
	if br != nil {
		_ = br.Stop(2 * time.Second)
	}
}
