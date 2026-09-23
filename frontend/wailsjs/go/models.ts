export namespace main {
	
	export class CoreStatus {
	    running: boolean;
	    starting: boolean;
	    nodeId: string;
	    nodeName: string;
	    endpoint: string;
	    socksPort: number;
	    httpPort: number;
	    startedAtMs: number;
	    uptimeMs: number;
	    lastError: string;
	
	    static createFrom(source: any = {}) {
	        return new CoreStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.starting = source["starting"];
	        this.nodeId = source["nodeId"];
	        this.nodeName = source["nodeName"];
	        this.endpoint = source["endpoint"];
	        this.socksPort = source["socksPort"];
	        this.httpPort = source["httpPort"];
	        this.startedAtMs = source["startedAtMs"];
	        this.uptimeMs = source["uptimeMs"];
	        this.lastError = source["lastError"];
	    }
	}
	export class ParseError {
	    line: number;
	    text: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ParseError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.line = source["line"];
	        this.text = source["text"];
	        this.error = source["error"];
	    }
	}
	export class NodeView {
	    id: string;
	    name: string;
	    protocol: string;
	    address: string;
	    port: number;
	    endpoint: string;
	    transport: string;
	    network: string;
	    security: string;
	    sni?: string;
	    host?: string;
	    path?: string;
	    flow?: string;
	    fp?: string;
	    pbk?: string;
	    sid?: string;
	    spx?: string;
	    headerType?: string;
	    alpn?: string;
	    encryption?: string;
	    allowInsecure: boolean;
	    certPin?: string;
	    certNote?: string;
	    notes?: string[];
	    pingMs: number;
	    pingError?: string;
	    pingAt?: number;
	    addedAt?: number;
	    selected: boolean;
	    connected: boolean;
	    pinging: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NodeView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.protocol = source["protocol"];
	        this.address = source["address"];
	        this.port = source["port"];
	        this.endpoint = source["endpoint"];
	        this.transport = source["transport"];
	        this.network = source["network"];
	        this.security = source["security"];
	        this.sni = source["sni"];
	        this.host = source["host"];
	        this.path = source["path"];
	        this.flow = source["flow"];
	        this.fp = source["fp"];
	        this.pbk = source["pbk"];
	        this.sid = source["sid"];
	        this.spx = source["spx"];
	        this.headerType = source["headerType"];
	        this.alpn = source["alpn"];
	        this.encryption = source["encryption"];
	        this.allowInsecure = source["allowInsecure"];
	        this.certPin = source["certPin"];
	        this.certNote = source["certNote"];
	        this.notes = source["notes"];
	        this.pingMs = source["pingMs"];
	        this.pingError = source["pingError"];
	        this.pingAt = source["pingAt"];
	        this.addedAt = source["addedAt"];
	        this.selected = source["selected"];
	        this.connected = source["connected"];
	        this.pinging = source["pinging"];
	    }
	}
	export class ImportResult {
	    added: NodeView[];
	    errors: ParseError[];
	    total: number;
	    existing: number;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.added = this.convertValues(source["added"], NodeView);
	        this.errors = this.convertValues(source["errors"], ParseError);
	        this.total = source["total"];
	        this.existing = source["existing"];
	        this.message = source["message"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LogEntry {
	    ts: number;
	    time: string;
	    level: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ts = source["ts"];
	        this.time = source["time"];
	        this.level = source["level"];
	        this.text = source["text"];
	    }
	}
	
	
	export class PingResult {
	    id: string;
	    name: string;
	    endpoint: string;
	    ok: boolean;
	    ms: number;
	    jitterMs?: number;
	    samples?: number;
	    error?: string;
	    at: number;
	
	    static createFrom(source: any = {}) {
	        return new PingResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.endpoint = source["endpoint"];
	        this.ok = source["ok"];
	        this.ms = source["ms"];
	        this.jitterMs = source["jitterMs"];
	        this.samples = source["samples"];
	        this.error = source["error"];
	        this.at = source["at"];
	    }
	}
	export class ProxyStatus {
	    supported: boolean;
	    enabledByApp: boolean;
	    proxyEnable: boolean;
	    proxyServer: string;
	    proxyOverride: string;
	    key: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.supported = source["supported"];
	        this.enabledByApp = source["enabledByApp"];
	        this.proxyEnable = source["proxyEnable"];
	        this.proxyServer = source["proxyServer"];
	        this.proxyOverride = source["proxyOverride"];
	        this.key = source["key"];
	        this.error = source["error"];
	    }
	}
	export class Settings {
	    socksPort: number;
	    httpPort: number;
	    pingTimeoutMs: number;
	    pingSamples: number;
	    systemProxy: boolean;
	    proxyBypass: string;
	    bypassLocal: boolean;
	    allowInsecureAll: boolean;
	    sniffing: boolean;
	    logLevel: string;
	    dnsServers: string;
	    minimizeToTray: boolean;
	    connectOnStartup: boolean;
	    autoPingOnStart: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.socksPort = source["socksPort"];
	        this.httpPort = source["httpPort"];
	        this.pingTimeoutMs = source["pingTimeoutMs"];
	        this.pingSamples = source["pingSamples"];
	        this.systemProxy = source["systemProxy"];
	        this.proxyBypass = source["proxyBypass"];
	        this.bypassLocal = source["bypassLocal"];
	        this.allowInsecureAll = source["allowInsecureAll"];
	        this.sniffing = source["sniffing"];
	        this.logLevel = source["logLevel"];
	        this.dnsServers = source["dnsServers"];
	        this.minimizeToTray = source["minimizeToTray"];
	        this.connectOnStartup = source["connectOnStartup"];
	        this.autoPingOnStart = source["autoPingOnStart"];
	    }
	}
	export class StateView {
	    appName: string;
	    appVersion: string;
	    coreVersion: string;
	    connected: boolean;
	    connecting: boolean;
	    elapsedMs: number;
	    startedAtMs: number;
	    current?: NodeView;
	    status: CoreStatus;
	    proxy: ProxyStatus;
	    settings: Settings;
	    pingRunning: boolean;
	    platform: string;
	    storePath: string;
	    error?: string;
	    warnings?: string[];
	
	    static createFrom(source: any = {}) {
	        return new StateView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.appName = source["appName"];
	        this.appVersion = source["appVersion"];
	        this.coreVersion = source["coreVersion"];
	        this.connected = source["connected"];
	        this.connecting = source["connecting"];
	        this.elapsedMs = source["elapsedMs"];
	        this.startedAtMs = source["startedAtMs"];
	        this.current = this.convertValues(source["current"], NodeView);
	        this.status = this.convertValues(source["status"], CoreStatus);
	        this.proxy = this.convertValues(source["proxy"], ProxyStatus);
	        this.settings = this.convertValues(source["settings"], Settings);
	        this.pingRunning = source["pingRunning"];
	        this.platform = source["platform"];
	        this.storePath = source["storePath"];
	        this.error = source["error"];
	        this.warnings = source["warnings"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

