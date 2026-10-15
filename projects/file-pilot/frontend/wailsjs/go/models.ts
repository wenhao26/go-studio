export namespace app {
	
	export class ActiveTask {
	    task_id: number;
	    type: string;
	    status: string;
	    scan_id: number;
	    root_path: string;
	    error: string;
	    elapsed_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new ActiveTask(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.type = source["type"];
	        this.status = source["status"];
	        this.scan_id = source["scan_id"];
	        this.root_path = source["root_path"];
	        this.error = source["error"];
	        this.elapsed_ms = source["elapsed_ms"];
	    }
	}
	export class CategoryCount {
	    category: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new CategoryCount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.category = source["category"];
	        this.count = source["count"];
	    }
	}
	export class FileItem {
	    id: number;
	    name: string;
	    path: string;
	    size_bytes: number;
	    category: string;
	    confidence?: number;
	    source: string;
	    reason: string;
	    excluded: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.size_bytes = source["size_bytes"];
	        this.category = source["category"];
	        this.confidence = source["confidence"];
	        this.source = source["source"];
	        this.reason = source["reason"];
	        this.excluded = source["excluded"];
	    }
	}
	export class FilePage {
	    items: FileItem[];
	    page: number;
	    page_size: number;
	    total_count: number;
	    total_page: number;
	
	    static createFrom(source: any = {}) {
	        return new FilePage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], FileItem);
	        this.page = source["page"];
	        this.page_size = source["page_size"];
	        this.total_count = source["total_count"];
	        this.total_page = source["total_page"];
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
	export class OrganizeFailure {
	    path: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new OrganizeFailure(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.reason = source["reason"];
	    }
	}
	export class OrganizeReport {
	    task_id: number;
	    status: string;
	    success_count: number;
	    failed_count: number;
	    duration_ms: number;
	    moved_size_bytes: number;
	    failures: OrganizeFailure[];
	
	    static createFrom(source: any = {}) {
	        return new OrganizeReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.status = source["status"];
	        this.success_count = source["success_count"];
	        this.failed_count = source["failed_count"];
	        this.duration_ms = source["duration_ms"];
	        this.moved_size_bytes = source["moved_size_bytes"];
	        this.failures = this.convertValues(source["failures"], OrganizeFailure);
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
	export class RecoveryNotice {
	    recovered_tasks: number;
	    recovered_scans: number;
	
	    static createFrom(source: any = {}) {
	        return new RecoveryNotice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.recovered_tasks = source["recovered_tasks"];
	        this.recovered_scans = source["recovered_scans"];
	    }
	}
	export class ScanSummary {
	    file_count: number;
	    dir_count: number;
	    total_size_bytes: number;
	    skipped_count: number;
	    duration_ms: number;
	    avg_speed: number;
	    max_workers: number;
	    status: string;
	    organizable_count: number;
	
	    static createFrom(source: any = {}) {
	        return new ScanSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file_count = source["file_count"];
	        this.dir_count = source["dir_count"];
	        this.total_size_bytes = source["total_size_bytes"];
	        this.skipped_count = source["skipped_count"];
	        this.duration_ms = source["duration_ms"];
	        this.avg_speed = source["avg_speed"];
	        this.max_workers = source["max_workers"];
	        this.status = source["status"];
	        this.organizable_count = source["organizable_count"];
	    }
	}
	export class Settings {
	    workers: number;
	    scan_hidden: boolean;
	    scan_system: boolean;
	    skip_large: boolean;
	    recent_dirs: string[];
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workers = source["workers"];
	        this.scan_hidden = source["scan_hidden"];
	        this.scan_system = source["scan_system"];
	        this.skip_large = source["skip_large"];
	        this.recent_dirs = source["recent_dirs"];
	    }
	}

}

export namespace scanner {
	
	export class Config {
	    workers: number;
	    scan_hidden: boolean;
	    scan_system: boolean;
	    skip_large: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workers = source["workers"];
	        this.scan_hidden = source["scan_hidden"];
	        this.scan_system = source["scan_system"];
	        this.skip_large = source["skip_large"];
	    }
	}

}

