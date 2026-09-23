package main

// xraydist.go — registers the Xray apps, proxies and transports this client
// actually needs. Xray is used as a plain Go library: every proxy/transport is
// an init() registration, so the set of blank imports below *is* the feature
// set of the built binary (mirrors xray-core's main/distro/all, trimmed down to
// keep the EXE small).

import (
	// Mandatory: dispatcher + inbound/outbound managers.
	_ "github.com/xtls/xray-core/app/dispatcher"
	_ "github.com/xtls/xray-core/app/proxyman/inbound"
	_ "github.com/xtls/xray-core/app/proxyman/outbound"

	// Optional apps used by the generated config.
	_ "github.com/xtls/xray-core/app/dns"
	_ "github.com/xtls/xray-core/app/log"
	_ "github.com/xtls/xray-core/app/policy"
	_ "github.com/xtls/xray-core/app/router"

	// Resolves the dependency cycle between core and transport/internet.
	_ "github.com/xtls/xray-core/transport/internet/tagged/taggedimpl"

	// Inbounds used by this client.
	_ "github.com/xtls/xray-core/proxy/http"
	_ "github.com/xtls/xray-core/proxy/socks"

	// Outbounds.
	_ "github.com/xtls/xray-core/proxy/blackhole"
	_ "github.com/xtls/xray-core/proxy/freedom"
	_ "github.com/xtls/xray-core/proxy/trojan"
	_ "github.com/xtls/xray-core/proxy/vless/outbound"

	// Transports.
	_ "github.com/xtls/xray-core/transport/internet/grpc"
	_ "github.com/xtls/xray-core/transport/internet/httpupgrade"
	_ "github.com/xtls/xray-core/transport/internet/reality"
	_ "github.com/xtls/xray-core/transport/internet/splithttp"
	_ "github.com/xtls/xray-core/transport/internet/tcp"
	_ "github.com/xtls/xray-core/transport/internet/tls"
	_ "github.com/xtls/xray-core/transport/internet/udp"
	_ "github.com/xtls/xray-core/transport/internet/websocket"

	// Transport headers (type=none / type=http).
	_ "github.com/xtls/xray-core/transport/internet/headers/http"
	_ "github.com/xtls/xray-core/transport/internet/headers/noop"

	// JSON config support for core.LoadConfig("json", ...).
	_ "github.com/xtls/xray-core/main/json"
)
