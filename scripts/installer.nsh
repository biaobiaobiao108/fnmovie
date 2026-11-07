!include "FileFunc.nsh"

; MyGo remembers InstallLocation for new installations. Recover the folder
; from the older installer's UninstallString on the first upgrade only.
!macro customInit
  ; NSIS removes /D from GetParameters, so inspect the original command line.
  System::Call 'kernel32::GetCommandLineW() w .r0'
  ClearErrors
  ${GetOptions} $0 "/D=" $1
  IfErrors 0 done

  ReadRegStr $0 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.fnmovie.desktop" "InstallLocation"
  StrCmp $0 "" 0 done
  ReadRegStr $0 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.fnmovie.desktop" "UninstallString"
  StrCmp $0 "" done
  StrCpy $1 $0 1
  StrCmp $1 '$\"' 0 no_leading_quote
    StrCpy $0 $0 "" 1
    StrCpy $1 $0 1 -1
    StrCmp $1 '$\"' 0 done
    StrCpy $0 $0 -1
no_leading_quote:
  StrCpy $1 $0 14 -14
  StrCmp $1 "\Uninstall.exe" 0 done
  StrCpy $1 $0 -14
  StrCmp $1 "" done
  StrCpy $INSTDIR $1
done:
!macroend
