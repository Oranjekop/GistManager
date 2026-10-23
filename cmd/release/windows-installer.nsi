Unicode true
ManifestDPIAware true
SetCompressor /SOLID lzma
Name @NAME@
OutFile @OUTPUT@
InstallDir "$LOCALAPPDATA\Programs\@DIRECTORY@"
RequestExecutionLevel user
BrandingText " "

!define MUI_ICON @ICON@
!define MUI_UNICON @ICON@
!define MUI_FINISHPAGE_RUN "$INSTDIR\@EXECUTABLE@"
!define MUI_FINISHPAGE_RUN_TEXT "运行 @DISPLAY_NAME@"
!define MUI_FINISHPAGE_SHOWREADME
!define MUI_FINISHPAGE_SHOWREADME_TEXT "创建桌面快捷方式"
!define MUI_FINISHPAGE_SHOWREADME_FUNCTION CreateDesktopShortcut
!include "MUI2.nsh"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"

Section
  SetOutPath "$INSTDIR"
@FILES@
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  CreateShortCut "$SMPROGRAMS\@DISPLAY_NAME@.lnk" "$INSTDIR\@EXECUTABLE@"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "DisplayName" @NAME@
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "DisplayVersion" @VERSION@
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "DisplayIcon" "$INSTDIR\@EXECUTABLE@"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "NoRepair" 1
SectionEnd

Function CreateDesktopShortcut
  SetOutPath "$INSTDIR"
  CreateShortCut "$DESKTOP\@DISPLAY_NAME@.lnk" "$INSTDIR\@EXECUTABLE@"
FunctionEnd

; Explorer can briefly hold a newly created shortcut open.
Function un.DeleteShortcut
  Exch $0
  Push $1
  StrCpy $1 50
  retry:
    ClearErrors
    Delete $0
    IfErrors 0 done
    IntOp $1 $1 - 1
    IntCmp $1 0 done
    Sleep 100
    Goto retry
  done:
  Pop $1
  Pop $0
FunctionEnd

Section "Uninstall"
  Push "$SMPROGRAMS\@DISPLAY_NAME@.lnk"
  Call un.DeleteShortcut
  Push "$DESKTOP\@DISPLAY_NAME@.lnk"
  Call un.DeleteShortcut
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@"
  RMDir /r "$INSTDIR"
SectionEnd
