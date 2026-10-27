Unicode true
!define REQUEST_EXECUTION_LEVEL "user"
!define UNINST_KEY_NAME "Yunbo"
!include "wails_tools.nsh"
!include "MUI2.nsh"
!include "LogicLib.nsh"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe"
InstallDir "$LOCALAPPDATA\Programs\Yunbo"
InstallDirRegKey HKCU "Software\Yunbo" "InstallDir"
SetCompressor /SOLID lzma
ManifestDPIAware true
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIAddVersionKey "ProductName" "${INFO_PRODUCTNAME}"
VIAddVersionKey "ProductVersion" "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion" "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileDescription" "Yunbo Setup"
VIAddVersionKey "LegalCopyright" "${INFO_COPYRIGHT}"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

Function .onInit
    !insertmacro wails.checkArchitecture
FunctionEnd

Section "Install"
    SetShellVarContext current
    !insertmacro wails.webview2runtime
    SetOutPath "$INSTDIR"
    !insertmacro wails.files
    WriteUninstaller "$INSTDIR\uninstall.exe"
    CreateShortcut "$SMPROGRAMS\云泊 Yunbo.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortcut "$DESKTOP\云泊 Yunbo.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    WriteRegStr HKCU "Software\Yunbo" "InstallDir" "$INSTDIR"
    SetRegView 64
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "${INFO_PRODUCTNAME}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${INFO_PRODUCTVERSION}"
    WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "${INFO_COMPANYNAME}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" '$\"$INSTDIR\uninstall.exe$\"'
    WriteRegStr HKCU "${UNINST_KEY}" "QuietUninstallString" '$\"$INSTDIR\uninstall.exe$\" /S'
    WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
    WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1
SectionEnd

Section "Uninstall"
    SetShellVarContext current
    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
    Delete "$INSTDIR\uninstall.exe"
    RMDir "$INSTDIR"
    Delete "$SMPROGRAMS\云泊 Yunbo.lnk"
    Delete "$DESKTOP\云泊 Yunbo.lnk"
    SetRegView 64
    DeleteRegKey HKCU "${UNINST_KEY}"
    DeleteRegKey HKCU "Software\Yunbo"
    ; Keep the user's connection settings, transfer checkpoints and downloads.
SectionEnd
