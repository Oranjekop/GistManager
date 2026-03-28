!macro customInstall
  Delete "$DESKTOP\Gist管理器.lnk"
  Delete "$SMPROGRAMS\Gist管理器.lnk"
  CreateShortCut "$DESKTOP\Gist管理器.lnk" "$INSTDIR\Gist管理器.exe" "" "$INSTDIR\build\icon.ico" 0
  CreateShortCut "$SMPROGRAMS\Gist管理器.lnk" "$INSTDIR\Gist管理器.exe" "" "$INSTDIR\build\icon.ico" 0
!macroend

!macro customUnInstall
  Delete "$DESKTOP\Gist管理器.lnk"
  Delete "$SMPROGRAMS\Gist管理器.lnk"
!macroend
