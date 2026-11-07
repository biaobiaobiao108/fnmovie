; MyGo v0.8.1 stores the install folder as InstallLocation and uses
; InstallDirRegKey on later installs. Older MyGo releases only wrote
; UninstallString, so recover the folder from it once during the first
; upgrade after this change.
!macro customInit
  ReadRegStr $0 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.fnmovie.desktop" "InstallLocation"
  StrCmp $0 "" custom_init_legacy

  ; InstallLocation may be written with surrounding quotes.
  StrCpy $1 $0 1
  StrCmp $1 '"' 0 custom_init_location_leading_done
    StrCpy $0 $0 -1 1
custom_init_location_leading_done:
  StrCpy $1 $0 1 -1
  StrCmp $1 '"' 0 custom_init_location_trailing_done
    StrCpy $0 $0 -1
custom_init_location_trailing_done:
  StrCmp $0 "" custom_init_legacy
  StrCpy $INSTDIR $0
  Goto custom_init_done

custom_init_legacy:
  ReadRegStr $0 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.fnmovie.desktop" "UninstallString"
  StrCmp $0 "" custom_init_done

  ; Remove the surrounding quotes written by the old installer.
  StrCpy $1 $0 1
  StrCmp $1 '"' 0 custom_init_leading_quote_done
    StrCpy $0 $0 -1 1
custom_init_leading_quote_done:
  StrCpy $1 $0 1 -1
  StrCmp $1 '"' 0 custom_init_trailing_quote_done
    StrCpy $0 $0 -1
custom_init_trailing_quote_done:

  ; UninstallString ends with \Uninstall.exe, which is 14 characters.
  StrCpy $1 $0 -14
  StrCmp $1 "" custom_init_done
    StrCpy $INSTDIR $1

custom_init_done:
!macroend
