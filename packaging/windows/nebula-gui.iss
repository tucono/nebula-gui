; @authorship: AI-Assisted
; @architect: AI (Claude Opus 5.5)
;
; The Windows installer, built with Inno Setup 6 by make package-windows,
; which defines AppVersion (the semver from packaging/version.sh) and
; NumVersion (its numeric part, for the file's version resource). Paths
; are relative to this file.
;
; It installs both programs and Wintun, adds the firewall rule, and has
; nebula-guid.exe create the nebula-gui group, give the person signed in
; access, and register the service. An upgrade keeps the service's
; registration, so it never grants access as a side effect.
;
; Silent use: setup /VERYSILENT /SUPPRESSMSGBOXES /NORESTART; unins000.exe
; the same, plus /REMOVESETTINGS to remove everything nebula-gui saved,
; every user's files included.

#ifndef AppVersion
  #error Build with make package-windows, which defines AppVersion and NumVersion.
#endif

#define Dist "..\..\dist"

[Setup]
AppId={{3BBF9D3F-6B96-4435-A8EB-EC9BF3D82E1A}
AppName=nebula-gui
AppVersion={#AppVersion}
AppVerName=nebula-gui {#AppVersion}
AppPublisher=nebula-gui
AppPublisherURL=https://github.com/tucono/nebula-gui
VersionInfoVersion={#NumVersion}
DefaultDirName={autopf}\nebula-gui
DisableProgramGroupPage=yes
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
OutputDir={#Dist}
OutputBaseFilename=nebula-gui-{#AppVersion}-windows-amd64-setup
SetupIconFile={#Dist}\icons\nebula-gui.ico
UninstallDisplayIcon={app}\nebula-gui.ico
UninstallDisplayName=nebula-gui
WizardStyle=modern
Compression=lzma2
SolidCompression=yes
; The app may be open, in the tray or in another person's session. It
; doesn't answer Restart Manager's request to close, so it's ended.
CloseApplications=force
CloseApplicationsFilter=nebula-gui.exe
RestartApplications=no

[Files]
Source: "{#Dist}\windows\nebula-gui.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Dist}\windows\nebula-guid.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Dist}\icons\nebula-gui.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Dist}\THIRD_PARTY_NOTICES.txt"; DestDir: "{app}"; Flags: ignoreversion
; Nebula looks for the DLL at this path under nebula-guid.exe's directory.
Source: "{#Dist}\windows\wintun\bin\amd64\wintun.dll"; DestDir: "{app}\dist\windows\wintun\bin\amd64"; Flags: ignoreversion
Source: "{#Dist}\windows\wintun\LICENSE.txt"; DestDir: "{app}\dist\windows\wintun"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\nebula-gui"; Filename: "{app}\nebula-gui.exe"; IconFilename: "{app}\nebula-gui.ico"

[Run]
Filename: "{app}\nebula-gui.exe"; Description: "Open nebula-gui"; Flags: postinstall nowait skipifsilent runasoriginaluser

[Code]
const
  ServiceName = 'nebula-guid';
  GroupName = 'nebula-gui';
  FirewallRule = 'nebula-gui';
  ErrServiceDoesNotExist = 1060;

var
  ServiceExisted: Boolean;
  ServiceWasRunning: Boolean;
  RemoveSettings: Boolean;

function Sys(Name: String): String;
begin
  Result := ExpandConstant('{sys}\' + Name);
end;

// RunHidden runs a program without a window, waits for it, and writes its
// output to the setup log. It returns the exit code, or -1 if it didn't
// start.
function RunHidden(Exe, Params: String): Integer;
var
  Code: Integer;
begin
  Log('Running: ' + Exe + ' ' + Params);
  if not ExecAndLogOutput(Exe, Params, '', SW_HIDE, ewWaitUntilTerminated, Code, nil) then
    Code := -1;
  Log(Format('Exit code %d', [Code]));
  Result := Code;
end;

function HasParam(Name: String): Boolean;
var
  I: Integer;
begin
  Result := False;
  for I := 1 to ParamCount do
    if CompareText(ParamStr(I), Name) = 0 then
      Result := True;
end;

// ServiceIsOurs reports whether the registered service runs this
// installation's nebula-guid.exe, rather than one registered by hand
// elsewhere during development.
function ServiceIsOurs(): Boolean;
var
  ImagePath: String;
begin
  Result := RegQueryStringValue(HKLM, 'SYSTEM\CurrentControlSet\Services\' + ServiceName,
      'ImagePath', ImagePath) and
    (Pos(Lowercase(ExpandConstant('{app}\nebula-guid.exe')), Lowercase(ImagePath)) > 0);
end;

// PrepareToInstall runs before any file is replaced. On an upgrade it
// stops the service, which stops Nebula, so nebula-guid.exe and
// wintun.dll can be replaced; it's started again afterwards only if it
// was running.
function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  Result := '';
  ServiceExisted := RunHidden(Sys('sc.exe'), 'query ' + ServiceName) <> ErrServiceDoesNotExist;
  // net stop waits until the service has stopped. It fails if it wasn't
  // running.
  if ServiceExisted then
    ServiceWasRunning := RunHidden(Sys('net.exe'), 'stop ' + ServiceName) = 0;
  if ServiceExisted and not ServiceIsOurs() then
  begin
    // Registered from somewhere else: replace it, as a fresh install.
    Log('The service runs another nebula-guid.exe; replacing it');
    RunHidden(Sys('sc.exe'), 'delete ' + ServiceName);
    ServiceExisted := False;
  end;
end;

procedure AddFirewallRule();
begin
  // Replaced on every install, so there's only ever one, for this path.
  RunHidden(Sys('netsh.exe'), 'advfirewall firewall delete rule name="' + FirewallRule + '"');
  if RunHidden(Sys('netsh.exe'), 'advfirewall firewall add rule name="' + FirewallRule +
      '" dir=in action=allow protocol=UDP enable=yes program="' +
      ExpandConstant('{app}\nebula-guid.exe') + '"') <> 0 then
    SuppressibleMsgBox('Couldn''t add the firewall rule for Nebula, so other computers ' +
      'may not be able to reach this one. The setup log has the details.',
      mbError, MB_OK, IDOK);
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  Code: Integer;
begin
  if CurStep <> ssPostInstall then
    Exit;
  AddFirewallRule();
  if not ServiceExisted then
  begin
    // Creates the group, gives the person signed in access at once and
    // adds them to the group, and registers the service.
    Code := RunHidden(ExpandConstant('{app}\nebula-guid.exe'), 'install --allow-session-user');
    if Code <> 0 then
    begin
      SuppressibleMsgBox(Format('Couldn''t set up the nebula-gui service (error %d). ' +
        'The setup log has the details.', [Code]), mbCriticalError, MB_OK, IDOK);
      Exit;
    end;
  end;
  if (not ServiceExisted) or ServiceWasRunning then
    // If this fails, the app says the service isn't running and offers
    // to start it.
    RunHidden(Sys('net.exe'), 'start ' + ServiceName);
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  case CurUninstallStep of
    usUninstall:
      begin
        RemoveSettings := HasParam('/REMOVESETTINGS') or
          (SuppressibleMsgBox('Remove everything nebula-gui saved on this computer?' + #13#10#13#10 +
            'This deletes the imported config, the service''s logs and the ' +
            'nebula-gui group, and every user''s saved peer names, app log and ' +
            'settings. Choose No to keep them for a reinstall.',
            mbConfirmation, MB_YESNO or MB_DEFBUTTON2, IDNO) = IDYES);
        // The app, in every session, so its files can be removed and it
        // can't write them again.
        RunHidden(Sys('taskkill.exe'), '/f /im nebula-gui.exe');
        // Every user's start-at-sign-in entry, which would otherwise point
        // at a missing program; and their files too if the answer was Yes.
        // A failure is in the setup log and doesn't stop the uninstall.
        if RemoveSettings then
          RunHidden(ExpandConstant('{app}\nebula-guid.exe'), 'remove-user-files')
        else
          RunHidden(ExpandConstant('{app}\nebula-guid.exe'), 'remove-user-files --autostart-only');
        // Stops Nebula and removes the service before its files go.
        RunHidden(ExpandConstant('{app}\nebula-guid.exe'), 'uninstall');
        RunHidden(Sys('netsh.exe'), 'advfirewall firewall delete rule name="' + FirewallRule + '"');
      end;
    usPostUninstall:
      if RemoveSettings then
      begin
        DelTree(ExpandConstant('{commonappdata}\nebula-gui'), True, True, True);
        RunHidden(Sys('net.exe'), 'localgroup ' + GroupName + ' /delete');
      end;
  end;
end;
