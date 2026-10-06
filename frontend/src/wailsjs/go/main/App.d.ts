import {main, config, ssh} from '../models';

export function CloseTab(arg1:string):Promise<boolean>;

export function GetFilteredLines(arg1:string,arg2:number,arg3:number):Promise<Array<main.LogLineResponse>>;

export function GetLogLines(arg1:string,arg2:number,arg3:number):Promise<Array<main.LogLineResponse>>;

export function GetRecentFiles():Promise<Array<config.RecentFileItem>>;

export function GetSavedSession():Promise<config.SavedSession>;

export function SaveSession(arg1:config.SavedSession):Promise<void>;

export function ClearSavedSession():Promise<void>;

export function RestoreTab(arg1:config.SavedTabItem):Promise<main.FileStatsResponse>;

export function SaveAppSettings(arg1:Record<string, any>):Promise<void>;

export function GetSettings():Promise<{[key: string]: any}>;

export function OpenFile(arg1:string):Promise<main.FileStatsResponse>;

export function OpenFileDialog():Promise<main.FileStatsResponse>;

export function OpenFiles(arg1:Array<string>):Promise<Array<main.FileStatsResponse>>;

export function OpenMultipleFilesDialog():Promise<Array<main.FileStatsResponse>>;

export function RenameTab(arg1:string,arg2:string):Promise<boolean>;

export function SaveSettings(arg1:string,arg2:string,arg3:boolean):Promise<void>;

export function SearchLines(arg1:string,arg2:string,arg3:boolean,arg4:boolean,arg5:boolean):Promise<number>;

export function SetActiveTab(arg1:string):Promise<boolean>;

export function SetAutoScroll(arg1:string,arg2:boolean):Promise<void>;

export function SaveFullLogAs(arg1:string):Promise<string>;

export function SaveSearchResults(arg1:string):Promise<string>;

export function ValidateRegex(arg1:string):Promise<main.RegexValidationResult>;

export function RemoveRecentFile(arg1:string):Promise<boolean>;

export function ClearRecentFiles():Promise<boolean>;

export function OpenSSHFile(arg1:main.SSHConfig):Promise<main.FileStatsResponse>;

export function AcceptSSHHost(arg1:string):Promise<main.FileStatsResponse>;

export function ListSSHFiles(arg1:main.SSHConfig,arg2:string):Promise<Array<ssh.DirEntry>>;

export function AckSSHHost(arg1:string):Promise<void>;

export function OpenRemoteFile(arg1:main.RemoteConfig):Promise<main.FileStatsResponse>;

export function ListRemoteFiles(arg1:main.RemoteConfig,arg2:string):Promise<Array<main.DirEntry>>;

export function AckFTPCert(arg1:string):Promise<void>;

export function AddInputHistory(arg1:string,arg2:string):Promise<void>;

export function ClearInputHistory(arg1:string):Promise<void>;

export function GetInputHistory(arg1:string):Promise<Array<string>>;

export function RemoveInputHistoryItem(arg1:string,arg2:string):Promise<void>;

export function SetMaxHistorySize(arg1:number):Promise<void>;

export function SaveSearchPaneHeight(arg1:number):Promise<void>;

export function GetSearchPaneHeight():Promise<number>;

export function SaveSearchPaneWidth(arg1:number):Promise<void>;

export function GetSearchPaneWidth():Promise<number>;

export function SaveSearchPaneLayout(arg1:string,arg2:string):Promise<void>;

export function GetSearchPaneLayout():Promise<{[key: string]: string}>;
