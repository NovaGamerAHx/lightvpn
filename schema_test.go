package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	conf "github.com/xtls/xray-core/infra/conf"
)

// Xray decodes its JSON config with encoding/json, so a key that is not a field
// of the corresponding conf struct is *silently dropped*: the option just stops
// existing, yet the config still validates. This test walks every generated
// config and rejects any key Xray would ignore, deriving the allowed names from
// Xray's own struct tags so it cannot go stale.
//
// It caught real problems during development (a legacy "settings.timeout" on both
// inbounds, an invented "domainMatcher" on routing).

func TestGeneratedConfigUsesOnlyRealXrayKeys(t *testing.T) {
	links := []string{
		vlessWS,
		vlessReality,
		trojanWS,
		trojanTCP,
		"vless://" + testUUID + "@h.example:2053?encryption=none&security=tls&type=grpc&serviceName=s&mode=multi&alpn=h2#grpc",
		"vless://" + testUUID + "@h.example:443?encryption=none&security=none&type=tcp&headerType=http#plain",
		"trojan://pw@h.example:443?security=tls&type=xhttp&mode=auto&path=%2Fxhttp#xhttp",
		"trojan://pw@h.example:443?security=tls&type=httpupgrade#httpupgrade",
		"vless://" + testUUID + "@h.example:443?encryption=none&security=reality&pbk=" + testPubKey + "&sid=00&spx=%2Fx&type=grpc&serviceName=x&fp=firefox#reality-grpc",
	}
	for _, raw := range links {
		l, err := ParseShareLink(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", clip(raw, 50), err)
		}
		cfg, err := BuildXrayConfig(l, DefaultCoreOptions())
		if err != nil {
			t.Fatalf("build %s: %v", l.Name, err)
		}
		t.Run(l.Name, func(t *testing.T) {
			var doc map[string]any
			if err := json.Unmarshal(cfg, &doc); err != nil {
				t.Fatal(err)
			}
			v := &validator{t: t}
			v.object("<root>", reflect.TypeOf(conf.Config{}), doc)
			if v.bad == 0 {
				return
			}
			t.Fatalf("%d unknown key(s) in the generated config", v.bad)
		})
	}
}

// rulesFields mirrors the field names xray's infra/conf/routing parseFieldRule
// understands; routing rules are decoded by hand, not from struct tags.
var rulesFields = map[string]bool{
	"domain": true, "type": true, "ip": true, "port": true, "sourcePort": true,
	"network": true, "source": true, "user": true, "protocol": true,
	"inboundTag": true, "outboundTag": true, "balancerTag": true,
}

type validator struct {
	t   *testing.T
	bad int
}

func (v *validator) object(path string, typ reflect.Type, obj map[string]any) {
	typ = deref(typ)
	if typ == nil || typ.Kind() != reflect.Struct {
		return
	}
	byTag := map[string]reflect.StructField{}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.PkgPath != "" {
			continue
		}
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name[:1]) + f.Name[1:]
		}
		byTag[strings.Split(name, ".")[0]] = f
	}

	for k, val := range obj {
		if k == "type" && strings.HasSuffix(path, ".rules[]") {
			continue // "type":"field" is the conventional marker, ignored by xray
		}
		f, known := byTag[k]
		if !known {
			v.bad++
			v.t.Errorf("%s: key %q is not a field of %s — Xray would ignore it", path, k, typeString(typ))
			continue
		}
		child := path + "." + k

		// Routing rules are parsed field-by-field instead of by a struct.
		if strings.HasSuffix(child, ".rules") {
			if arr, ok := val.([]any); ok {
				for _, e := range arr {
					o, ok := e.(map[string]any)
					if !ok {
						continue
					}
					for kk := range o {
						if !rulesFields[kk] {
							v.bad++
							v.t.Errorf("%s[]: key %q is not a routing rule field", child, kk)
						}
					}
				}
			}
			continue
		}

		switch ft := f.Type; {
		case ft.Kind() == reflect.Slice || ft.Kind() == reflect.Array:
			elem := deref(ft.Elem())
			arr, ok := val.([]any)
			if !ok {
				continue
			}
			for _, e := range arr {
				o, ok := e.(map[string]any)
				if !ok {
					continue
				}
				if elem == reflect.TypeOf(json.RawMessage{}) {
					// "users" of a vless vnext entry is decoded by hand
					if typ.Name() == "VLessOutboundVnext" {
						v.object(child+"[]", reflect.TypeOf(conf.VLessOutboundConfig{}), o)
					}
					continue
				}
				v.object(child+"[]", elem, o)
			}
		case ft.Kind() == reflect.Struct || (ft.Kind() == reflect.Ptr && deref(ft) != nil && deref(ft).Kind() == reflect.Struct):
			if o, ok := val.(map[string]any); ok {
				if deref(ft).Kind() == reflect.Struct && ft.String() != "json.RawMessage" && ft.String() != "*json.RawMessage" {
					v.object(child, deref(ft), o)
				}
			}
		}
	}
}

func TestInboundAndOutboundSettingsKeys(t *testing.T) {
	for _, raw := range []string{vlessWS, trojanTCP, vlessReality} {
		l, err := ParseShareLink(raw)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := BuildXrayConfig(l, DefaultCoreOptions())
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(cfg, &doc); err != nil {
			t.Fatal(err)
		}
		for i, e := range doc["inbounds"].([]any) {
			inb := e.(map[string]any)
			settings := inb["settings"].(map[string]any)
			var typ reflect.Type
			switch inb["protocol"] {
			case "socks":
				typ = reflect.TypeOf(conf.SocksServerConfig{})
			case "http":
				typ = reflect.TypeOf(conf.HTTPServerConfig{})
			default:
				t.Fatalf("unexpected inbound protocol %v", inb["protocol"])
			}
			v := &validator{t: t}
			v.object(fmt.Sprintf("inbounds[%d].settings", i), typ, settings)
			if v.bad > 0 {
				t.Fatalf("%d unknown key(s) in the %v inbound settings", v.bad, inb["protocol"])
			}
		}
		outb := doc["outbounds"].([]any)[0].(map[string]any)
		settings := outb["settings"].(map[string]any)
		v := &validator{t: t}
		switch outb["protocol"] {
		case "vless":
			v.object("outbounds[0].settings", reflect.TypeOf(conf.VLessOutboundConfig{}), settings)
		case "trojan":
			v.object("outbounds[0].settings", reflect.TypeOf(conf.TrojanClientConfig{}), settings)
			// outbounds use TrojanServerTarget; TrojanServerConfig is the *inbound*
			for i, e := range settings["servers"].([]any) {
				v.object(fmt.Sprintf("outbounds[0].settings.servers[%d]", i),
					reflect.TypeOf(conf.TrojanServerTarget{}), e.(map[string]any))
			}
		default:
			t.Fatalf("unexpected outbound protocol %v", outb["protocol"])
		}
		if v.bad > 0 {
			t.Fatalf("%d unknown key(s) in the %v outbound settings", v.bad, outb["protocol"])
		}
	}
}

func deref(t reflect.Type) reflect.Type {
	if t == nil {
		return nil
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func typeString(t reflect.Type) string {
	if t == nil {
		return "<unknown>"
	}
	return "xray " + t.PkgPath() + "." + t.Name()
}
