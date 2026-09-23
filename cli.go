package main

// cli.go — the same engine, no window. Useful for three things:
//
//	LightVPN.exe --selftest     prove the tunnel works on this machine
//	LightVPN.exe --parse LINK   debug a share link that refuses to import
//	LightVPN.exe --render LINK  print the exact Xray JSON that would be used
//
// main.go (Windows) calls runCLI first and only starts Wails if it returns false.

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

const cliUsage = appName + " " + appVersion + " — a lightweight Xray client (no xray.exe, no installer)\n" + `
Usage:
  ` + appName + `.exe                     start the window (Windows)
  ` + appName + `.exe --selftest          run the built-in end-to-end tunnel test
  ` + appName + `.exe --parse <link>      show how a share link is parsed (JSON)
  ` + appName + `.exe --render <link>     print the generated Xray config (JSON)
  ` + appName + `.exe --ping <host:port>  measure TCP connect latency
  ` + appName + `.exe --import <file|->   import links from a file or stdin
  ` + appName + `.exe --serve [--node id] connect headless (Ctrl+C to stop)
  ` + appName + `.exe --config <path>     use an alternative config.json
  ` + appName + `.exe --version | --help

Options for --serve / --selftest:
  --node <id|name>   node to connect to (default: the selected one in config.json)
  --socks <port>     override the SOCKS5 port
  --http <port>      override the HTTP proxy port
  --proxy            also set the Windows system proxy while serving
`

func runCLI(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Print(cliUsage)
		return true
	case "-v", "--version", "version":
		fmt.Printf("%s %s · xray-core %s · %s/%s\n", appName, appVersion, xrayVersionString(), goos(), goarch())
		return true
	}

	fs := flag.NewFlagSet(appName, flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	var (
		selftest   = fs.Bool("selftest", false, "run the end-to-end tunnel self test")
		parse      = fs.String("parse", "", "parse one share link and print JSON")
		render     = fs.String("render", "", "parse a share link and print the generated Xray JSON")
		ping       = fs.String("ping", "", "TCP ping host:port")
		importFile = fs.String("import", "", "import share links from a file ('-' for stdin)")
		serve      = fs.Bool("serve", false, "connect headless (no window)")
		configPath = fs.String("config", "", "path to config.json (default: next to the EXE)")
		node       = fs.String("node", "", "node id or name")
		socksPort  = fs.Int("socks", 0, "SOCKS5 port override")
		httpPort   = fs.Int("http", 0, "HTTP proxy port override")
		setProxy   = fs.Bool("proxy", false, "apply the Windows system proxy")
		quiet      = fs.Bool("quiet", false, "only print the final verdict")
	)
	if err := fs.Parse(args); err != nil {
		return true
	}
	logf := func(level, msg string) {
		if !*quiet {
			fmt.Printf("  [%s] %s\n", level, msg)
		}
	}

	switch {
	case *parse != "":
		l, err := ParseShareLink(*parse)
		if err != nil {
			fail("parse failed: %v", err)
		}
		dumpJSON(l)
		return true

	case *render != "":
		l, err := ParseShareLink(*render)
		if err != nil {
			fail("parse failed: %v", err)
		}
		s := DefaultSettings()
		if *socksPort > 0 {
			s.SocksPort = *socksPort
		}
		if *httpPort > 0 {
			s.HTTPPort = *httpPort
		}
		cfg, err := BuildXrayConfig(l, s.coreOptions())
		if err != nil {
			fail("config generation failed: %v", err)
		}
		fmt.Println(string(cfg))
		return true

	case *ping != "":
		addr := *ping
		if !strings.Contains(addr, ":") {
			addr += ":443"
		}
		best, jitter, err := PingEndpoint(context.Background(), addr, 3*time.Second, 3)
		if err != nil {
			fail("%s unreachable: %s", addr, friendlyDialError(err))
		}
		fmt.Printf("%s  %.2f ms (jitter %.2f ms)\n", addr, roundMS(best), roundMS(jitter))
		return true

	case *importFile != "":
		data, err := readMaybeStdin(*importFile)
		if err != nil {
			fail("%v", err)
		}
		app, cleanup, err := NewApp(*configPath)
		if err != nil {
			fail("%v", err)
		}
		defer cleanup()
		res, err := app.ImportLinks(string(data))
		if err != nil {
			fail("%v", err)
		}
		fmt.Println(res.Message)
		for _, n := range res.Added {
			fmt.Printf("  + %-28s %s %s:%d  %s\n", clip(n.Name, 28), n.Protocol, n.Address, n.Port, n.Transport)
		}
		for _, e := range res.Errors {
			fmt.Printf("  ! line %d: %s (%s)\n", e.Line, e.Err, e.Text)
		}
		fmt.Printf("config: %s\n", app.ConfigPath())
		return true

	case *selftest:
		rep := RunSelfTest(logf)
		fmt.Println()
		for _, r := range rep.Results {
			mark := "FAIL"
			if r.OK {
				mark = "PASS"
			}
			fmt.Printf("%s  %-46s %6.0f ms  %s\n", mark, r.Name, r.MS, clip(r.Detail, 70))
		}
		fmt.Println("\n" + rep.Summary)
		if !rep.OK {
			os.Exit(1)
		}
		return true

	case *serve:
		return runServe(*configPath, *node, *socksPort, *httpPort, *setProxy, logf)
	}
	return false
}

func runServe(configPath, node string, socksPort, httpPort int, setProxy bool, logf func(string, string)) bool {
	app, cleanup, err := NewApp(configPath)
	if err != nil {
		fail("%v", err)
	}
	defer cleanup()

	if socksPort > 0 || httpPort > 0 || setProxy {
		s := app.store.Snapshot().Settings
		if socksPort > 0 {
			s.SocksPort = socksPort
		}
		if httpPort > 0 {
			s.HTTPPort = httpPort
		}
		s.SystemProxy = setProxy
		if _, err := app.UpdateSettings(s); err != nil {
			fail("%v", err)
		}
	}

	target, err := app.resolveNode(node)
	if err != nil {
		fail("%v", err)
	}
	if _, err := app.Connect(target); err != nil {
		fail("connect failed: %v", err)
	}
	st := app.State()
	fmt.Printf("connected: %s\n  socks5 127.0.0.1:%d\n  http   127.0.0.1:%d\n  Ctrl+C to disconnect\n",
		st.Current.Name, st.Settings.SocksPort, st.Settings.HTTPPort)
	if err := waitReady(fmt.Sprintf("127.0.0.1:%d", st.Settings.SocksPort), 5*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "warning: the SOCKS5 listener did not come up: %v\n", err)
	}
	if px := st.Proxy; px.ProxyEnable {
		fmt.Printf("  system proxy: %s\n", px.ProxyServer)
	}

	<-waitForInterrupt()
	fmt.Println("shutting down…")
	return true
}

// resolveNode maps a "" / id / name to a node id.
func (a *App) resolveNode(want string) (string, error) {
	d := a.store.Snapshot()
	if want == "" {
		if d.SelectedID != "" {
			return d.SelectedID, nil
		}
		if len(d.Configs) > 0 {
			return d.Configs[0].ID, nil
		}
		return "", fmt.Errorf("no nodes imported yet (use --import file)")
	}
	for _, c := range d.Configs {
		if c.ID == want || strings.EqualFold(c.Name, want) {
			return c.ID, nil
		}
	}
	return "", fmt.Errorf("no node with id or name %q", want)
}

func readMaybeStdin(path string) ([]byte, error) {
	if path == "-" {
		b, err := readAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("cannot read stdin: %w", err)
		}
		return b, nil
	}
	return os.ReadFile(path)
}

func dumpJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fail("%v", err)
	}
	fmt.Println(string(b))
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", a...)
	os.Exit(1)
}

func goos() string   { return runtimeGOOS }
func goarch() string { return runtimeGOARCH }
