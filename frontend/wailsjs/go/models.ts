export namespace main {
	
	export class ExportState {
	    name: string;
	    fps: number;
	    loop: boolean;
	    frames: string[];
	
	    static createFrom(source: any = {}) {
	        return new ExportState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.fps = source["fps"];
	        this.loop = source["loop"];
	        this.frames = source["frames"];
	    }
	}
	export class ExportArgs {
	    character: string;
	    cellSize: number;
	    states: ExportState[];
	
	    static createFrom(source: any = {}) {
	        return new ExportArgs(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.character = source["character"];
	        this.cellSize = source["cellSize"];
	        this.states = this.convertValues(source["states"], ExportState);
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
	
	export class GalleryImage {
	    name: string;
	    path: string;
	    size: number;
	    modTime: number;
	
	    static createFrom(source: any = {}) {
	        return new GalleryImage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
	    }
	}
	export class GenerateCharacterArgs {
	    description: string;
	    styleKey: string;
	    styleCustom: string;
	
	    static createFrom(source: any = {}) {
	        return new GenerateCharacterArgs(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.description = source["description"];
	        this.styleKey = source["styleKey"];
	        this.styleCustom = source["styleCustom"];
	    }
	}
	export class GenerateStateArgs {
	    baseImage: string;
	    description: string;
	    styleKey: string;
	    styleCustom: string;
	    cellSize: number;
	    safeMargin: number;
	    feedback: string;
	    refStrip: string;
	    state: sprite.StateSpec;
	
	    static createFrom(source: any = {}) {
	        return new GenerateStateArgs(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.baseImage = source["baseImage"];
	        this.description = source["description"];
	        this.styleKey = source["styleKey"];
	        this.styleCustom = source["styleCustom"];
	        this.cellSize = source["cellSize"];
	        this.safeMargin = source["safeMargin"];
	        this.feedback = source["feedback"];
	        this.refStrip = source["refStrip"];
	        this.state = this.convertValues(source["state"], sprite.StateSpec);
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
	export class ProviderInfo {
	    hasKey: boolean;
	    keyPreview: string;
	    model: string;
	    models: string[];
	
	    static createFrom(source: any = {}) {
	        return new ProviderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hasKey = source["hasKey"];
	        this.keyPreview = source["keyPreview"];
	        this.model = source["model"];
	        this.models = source["models"];
	    }
	}
	export class SettingsInfo {
	    provider: string;
	    providers: Record<string, ProviderInfo>;
	
	    static createFrom(source: any = {}) {
	        return new SettingsInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.providers = this.convertValues(source["providers"], ProviderInfo, true);
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
	export class StateResult {
	    name: string;
	    rawStrip: string;
	    frames: string[];
	    expected: number;
	    found: number;
	    warnings: string[];
	    scores: sprite.ScoreResult;
	
	    static createFrom(source: any = {}) {
	        return new StateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.rawStrip = source["rawStrip"];
	        this.frames = source["frames"];
	        this.expected = source["expected"];
	        this.found = source["found"];
	        this.warnings = source["warnings"];
	        this.scores = this.convertValues(source["scores"], sprite.ScoreResult);
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
	export class UpdateInfo {
	    available: boolean;
	    current: string;
	    latest: string;
	    notes: string;
	    canSelfUpdate: boolean;
	    downloadUrl: string;
	    assetSize: number;
	    err?: string;

	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.notes = source["notes"];
	        this.canSelfUpdate = source["canSelfUpdate"];
	        this.downloadUrl = source["downloadUrl"];
	        this.assetSize = source["assetSize"];
	        this.err = source["err"];
	    }
	}

}

export namespace sprite {
	
	export class DirectionInfo {
	    key: string;
	    label: string;
	    short: string;
	    mirrorOf: string;
	    row: number;
	    col: number;
	
	    static createFrom(source: any = {}) {
	        return new DirectionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.short = source["short"];
	        this.mirrorOf = source["mirrorOf"];
	        this.row = source["row"];
	        this.col = source["col"];
	    }
	}
	export class PresetInfo {
	    name: string;
	    label: string;
	    category: string;
	    action: string;
	    frames: number;
	    fps: number;
	    loop: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PresetInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.label = source["label"];
	        this.category = source["category"];
	        this.action = source["action"];
	        this.frames = source["frames"];
	        this.fps = source["fps"];
	        this.loop = source["loop"];
	    }
	}
	export class ScoreResult {
	    identity: number;
	    motion: number;
	    contact: number;
	    overall: number;
	
	    static createFrom(source: any = {}) {
	        return new ScoreResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.identity = source["identity"];
	        this.motion = source["motion"];
	        this.contact = source["contact"];
	        this.overall = source["overall"];
	    }
	}
	export class StateSpec {
	    name: string;
	    frames: number;
	    fps: number;
	    loop: boolean;
	    action: string;
	    facing: string;
	
	    static createFrom(source: any = {}) {
	        return new StateSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.frames = source["frames"];
	        this.fps = source["fps"];
	        this.loop = source["loop"];
	        this.action = source["action"];
	        this.facing = source["facing"];
	    }
	}

}

