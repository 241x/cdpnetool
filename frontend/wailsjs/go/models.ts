export namespace gui {
	
	export class ConfigListResult {
	    configs: storage.ConfigRecord[];
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigListResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configs = this.convertValues(source["configs"], storage.ConfigRecord);
	        this.success = source["success"];
	        this.error = source["error"];
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
	export class ConfigResult {
	    config?: storage.ConfigRecord;
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], storage.ConfigRecord);
	        this.success = source["success"];
	        this.error = source["error"];
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
	export class LaunchBrowserResult {
	    devToolsUrl: string;
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new LaunchBrowserResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.devToolsUrl = source["devToolsUrl"];
	        this.success = source["success"];
	        this.error = source["error"];
	    }
	}
	export class MatchedEventHistoryResult {
	    events: storage.MatchedEventRecord[];
	    total: number;
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new MatchedEventHistoryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.events = this.convertValues(source["events"], storage.MatchedEventRecord);
	        this.total = source["total"];
	        this.success = source["success"];
	        this.error = source["error"];
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
	export class NewConfigResult {
	    config?: storage.ConfigRecord;
	    configJson: string;
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new NewConfigResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], storage.ConfigRecord);
	        this.configJson = source["configJson"];
	        this.success = source["success"];
	        this.error = source["error"];
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
	export class NewRuleResult {
	    ruleJson: string;
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new NewRuleResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ruleJson = source["ruleJson"];
	        this.success = source["success"];
	        this.error = source["error"];
	    }
	}
	export class OperationResult {
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new OperationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.error = source["error"];
	    }
	}
	export class SessionResult {
	    sessionId: string;
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.success = source["success"];
	        this.error = source["error"];
	    }
	}
	export class SettingsResult {
	    settings: Record<string, string>;
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new SettingsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.settings = source["settings"];
	        this.success = source["success"];
	        this.error = source["error"];
	    }
	}
	export class StatsResult {
	    stats: model.EngineStats;
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new StatsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stats = this.convertValues(source["stats"], model.EngineStats);
	        this.success = source["success"];
	        this.error = source["error"];
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
	export class TargetListResult {
	    targets: model.TargetInfo[];
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new TargetListResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.targets = this.convertValues(source["targets"], model.TargetInfo);
	        this.success = source["success"];
	        this.error = source["error"];
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

export namespace model {
	
	export class EngineStats {
	    total: number;
	    matched: number;
	    byRule: Record<string, number>;
	
	    static createFrom(source: any = {}) {
	        return new EngineStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.matched = source["matched"];
	        this.byRule = source["byRule"];
	    }
	}
	export class TargetInfo {
	    id: string;
	    type: string;
	    url: string;
	    title: string;
	    isCurrent: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TargetInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.url = source["url"];
	        this.title = source["title"];
	        this.isCurrent = source["isCurrent"];
	    }
	}

}

export namespace storage {
	
	export class ConfigRecord {
	    id: number;
	    configId: string;
	    name: string;
	    version: string;
	    configJson: string;
	    isActive: boolean;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new ConfigRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.configId = source["configId"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.configJson = source["configJson"];
	        this.isActive = source["isActive"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	export class MatchedEventRecord {
	    id: number;
	    sessionId: string;
	    targetId: string;
	    url: string;
	    method: string;
	    stage: string;
	    statusCode: number;
	    finalResult: string;
	    matchedRulesJson: string;
	    originalJson: string;
	    modifiedJson: string;
	    timestamp: number;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new MatchedEventRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.sessionId = source["sessionId"];
	        this.targetId = source["targetId"];
	        this.url = source["url"];
	        this.method = source["method"];
	        this.stage = source["stage"];
	        this.statusCode = source["statusCode"];
	        this.finalResult = source["finalResult"];
	        this.matchedRulesJson = source["matchedRulesJson"];
	        this.originalJson = source["originalJson"];
	        this.modifiedJson = source["modifiedJson"];
	        this.timestamp = source["timestamp"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
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

