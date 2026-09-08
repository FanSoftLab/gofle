export namespace main {
	
	export class RemoteConfig {
	    protocol: string;
	    host: string;
	    port: number;
	    username: string;
	    auth_type: string;
	    password: string;
	    key_path: string;
	    key_passphrase: string;
	    remote_file_path: string;
	    tls_mode: string;

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.protocol = source["protocol"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.auth_type = source["auth_type"];
	        this.password = source["password"];
	        this.key_path = source["key_path"];
	        this.key_passphrase = source["key_passphrase"];
	        this.remote_file_path = source["remote_file_path"];
	        this.tls_mode = source["tls_mode"] || "none";
	    }
	}

	export class DirEntry {
	    name: string;
	    path: string;
	    kind: string;
	    size: number;
	    mod_time: number;

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.kind = source["kind"];
	        this.size = source["size"];
	        this.mod_time = source["mod_time"];
	    }
	}

	export class FileStatsResponse {
	    tab_id: string;
	    file_path: string;
	    file_name: string;
	    file_size: number;
	    total_lines: number;
	    matched_lines: number;
	    is_ssh: boolean;
	    is_ftp: boolean;
	    ssh?: config.SSHConfigSummary;

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tab_id = source["tab_id"];
	        this.file_path = source["file_path"];
	        this.file_name = source["file_name"];
	        this.file_size = source["file_size"];
	        this.total_lines = source["total_lines"];
	        this.matched_lines = source["matched_lines"];
	        this.is_ssh = source["is_ssh"];
	        this.is_ftp = source["is_ftp"];
	        this.ssh = source["ssh"] ? new config.SSHConfigSummary(source["ssh"]) : undefined;
	    }
	}

	export class LogLineResponse {
	    index: number;
	    content: string;

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.content = source["content"];
	    }
	}

	export class RegexValidationResult {
	    is_valid: boolean;
	    error_message: string;

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.is_valid = source["is_valid"];
	        this.error_message = source["error_message"];
	    }
	}

	export class SSHConfig {
	    host: string;
	    port: number;
	    username: string;
	    auth_type: string;
	    password: string;
	    key_path: string;
	    key_passphrase: string;
	    remote_file_path: string;

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.auth_type = source["auth_type"];
	        this.password = source["password"];
	        this.key_path = source["key_path"];
	        this.key_passphrase = source["key_passphrase"];
	        this.remote_file_path = source["remote_file_path"];
	    }
	}
}

export namespace config {
	
	export class RecentFileItem {
	    path: string;
	    file_size: number;
	    last_opened: string;
	    is_ssh: boolean;
	    ssh?: SSHConfigSummary;

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.file_size = source["file_size"];
	        this.last_opened = source["last_opened"];
	        this.is_ssh = source["is_ssh"];
	        this.ssh = this.convertValues(source["ssh"], SSHConfigSummary);
	    }

	    convertValues(a: any, classs: any): any {
	        if (!a) return a;
	        return new classs(a);
	    }
	}

	export class SSHConfigSummary {
	    host: string;
	    port: number;
	    username: string;
	    remote_file_path: string;
	    auth_type: string;
	    password: string;
	    key_path: string;
	    key_passphrase: string;
	    protocol?: string;
	    tls_mode?: string;

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.remote_file_path = source["remote_file_path"];
	        this.auth_type = source["auth_type"];
	        this.password = source["password"];
	        this.key_path = source["key_path"];
	        this.key_passphrase = source["key_passphrase"];
	        this.protocol = source["protocol"];
	        this.tls_mode = source["tls_mode"];
	    }
	}

	export class SavedTabItem {
	    id: string;
	    title: string;
	    path: string;
	    is_ssh: boolean;
	    ssh?: SSHConfigSummary;
	    group_id: string;

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.path = source["path"];
	        this.is_ssh = source["is_ssh"];
	        this.ssh = this.convertValues(source["ssh"], SSHConfigSummary);
	        this.group_id = source["group_id"];
	    }

	    convertValues(a: any, classs: any): any {
	        if (!a) return a;
	        return new classs(a);
	    }
	}

	export class SavedGroupItem {
	    id: string;
	    title: string;
	    color: string;
	    collapsed: boolean;
	    member_ids: string[];

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.color = source["color"];
	        this.collapsed = source["collapsed"];
	        this.member_ids = source["member_ids"];
	    }
	}

	export class SavedTopEntry {
	    type: string;
	    id: string;

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.id = source["id"];
	    }
	}

	export class SavedSession {
	    active_tab_id: string;
	    top_level: SavedTopEntry[];
	    tabs: SavedTabItem[];
	    groups: SavedGroupItem[];

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active_tab_id = source["active_tab_id"];
	        this.top_level = this.convertList(source["top_level"], SavedTopEntry);
	        this.tabs = this.convertList(source["tabs"], SavedTabItem);
	        this.groups = this.convertList(source["groups"], SavedGroupItem);
	    }

	    convertList(a: any, classs: any): any {
	        if (!a) return [];
	        return a.map((elem: any) => new classs(elem));
	    }
	}
}

export namespace ssh {
	
	export class DirEntry {
	    name: string;
	    path: string;
	    kind: string;
	    size: number;
	    mod_time: number;

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.kind = source["kind"];
	        this.size = source["size"];
	        this.mod_time = source["mod_time"];
	    }
	}
}
