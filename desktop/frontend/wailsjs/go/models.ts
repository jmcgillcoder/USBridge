export namespace androidclient {
	
	export class PublicIPStatus {
	    ipv4?: string;
	    ipv6?: string;
	
	    static createFrom(source: any = {}) {
	        return new PublicIPStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ipv4 = source["ipv4"];
	        this.ipv6 = source["ipv6"];
	    }
	}
	export class OperationResponse {
	    ok: boolean;
	    message: string;
	    before?: PublicIPStatus;
	    after?: PublicIPStatus;
	    commandSucceeded?: boolean;
	    networkDisconnected?: boolean;
	    networkRecovered?: boolean;
	    ipChanged?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new OperationResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	        this.before = this.convertValues(source["before"], PublicIPStatus);
	        this.after = this.convertValues(source["after"], PublicIPStatus);
	        this.commandSucceeded = source["commandSucceeded"];
	        this.networkDisconnected = source["networkDisconnected"];
	        this.networkRecovered = source["networkRecovered"];
	        this.ipChanged = source["ipChanged"];
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

export namespace main {
	
	export class AuthenticatedProxyAccess {
	    listen: string;
	    username: string;
	    password: string;
	    httpUrl: string;
	    socks5Url: string;
	
	    static createFrom(source: any = {}) {
	        return new AuthenticatedProxyAccess(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.listen = source["listen"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.httpUrl = source["httpUrl"];
	        this.socks5Url = source["socks5Url"];
	    }
	}
	export class DesktopSnapshot {
	    adapters: adapter.Adapter[];
	    // Go type: adapter
	    selectedAdapter?: any;
	    ipMode: string;
	    androidEndpoint?: string;
	    androidReady: boolean;
	    // Go type: androidclient
	    androidStatus?: any;
	    androidError?: string;
	    // Go type: time
	    androidCheckedAt?: any;
	    proxyListen: string;
	    // Go type: traffic
	    traffic: any;
	    proxyRunning: boolean;
	    networkChanging: boolean;
	    lastError?: string;
	    controlListen: string;
	    controlRunning: boolean;
	    controlError?: string;
	    authenticatedProxyListen: string;
	    authenticatedProxyRunning: boolean;
	    authenticatedProxyError?: string;
	    exclusiveModeSupported: boolean;
	    exclusiveModeEnabled: boolean;
	    exclusiveModeActive: boolean;
	    exclusiveModeInterface?: string;
	    exclusiveModeError?: string;
	    systemProxyActive: boolean;
	    systemProxyError?: string;
	
	    static createFrom(source: any = {}) {
	        return new DesktopSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.adapters = this.convertValues(source["adapters"], adapter.Adapter);
	        this.selectedAdapter = this.convertValues(source["selectedAdapter"], null);
	        this.ipMode = source["ipMode"];
	        this.androidEndpoint = source["androidEndpoint"];
	        this.androidReady = source["androidReady"];
	        this.androidStatus = this.convertValues(source["androidStatus"], null);
	        this.androidError = source["androidError"];
	        this.androidCheckedAt = this.convertValues(source["androidCheckedAt"], null);
	        this.proxyListen = source["proxyListen"];
	        this.traffic = this.convertValues(source["traffic"], null);
	        this.proxyRunning = source["proxyRunning"];
	        this.networkChanging = source["networkChanging"];
	        this.lastError = source["lastError"];
	        this.controlListen = source["controlListen"];
	        this.controlRunning = source["controlRunning"];
	        this.controlError = source["controlError"];
	        this.authenticatedProxyListen = source["authenticatedProxyListen"];
	        this.authenticatedProxyRunning = source["authenticatedProxyRunning"];
	        this.authenticatedProxyError = source["authenticatedProxyError"];
	        this.exclusiveModeSupported = source["exclusiveModeSupported"];
	        this.exclusiveModeEnabled = source["exclusiveModeEnabled"];
	        this.exclusiveModeActive = source["exclusiveModeActive"];
	        this.exclusiveModeInterface = source["exclusiveModeInterface"];
	        this.exclusiveModeError = source["exclusiveModeError"];
	        this.systemProxyActive = source["systemProxyActive"];
	        this.systemProxyError = source["systemProxyError"];
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

export namespace updater {

	export class Info {
	    currentVersion: string;
	    latestVersion: string;
	    available: boolean;
	    name?: string;
	    notes?: string;
	    releaseUrl?: string;
	    publishedAt?: string;

	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currentVersion = source["currentVersion"];
	        this.latestVersion = source["latestVersion"];
	        this.available = source["available"];
	        this.name = source["name"];
	        this.notes = source["notes"];
	        this.releaseUrl = source["releaseUrl"];
	        this.publishedAt = source["publishedAt"];
	    }
	}

}
