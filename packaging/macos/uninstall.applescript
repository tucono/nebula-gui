-- @authorship: AI-Assisted
-- @architect: AI (Claude Opus 5.5)
--
-- Uninstall nebula-gui.app: build-pkg.sh compiles it with osacompile and
-- the package puts it in /Applications, so nebula-gui can be removed
-- without Terminal. It asks what to remove and runs uninstall.sh as an
-- administrator, which removes this app too. Help > Uninstall nebula-gui...
-- in the app opens it. Keep this file ASCII: osacompile may not read UTF-8.

property uninstaller : "/usr/local/libexec/nebula-gui/uninstall.sh"
property keepButton : "Keep settings and uninstall"
property everythingButton : "Remove everything and uninstall"

on run
	try
		do shell script "test -x " & quoted form of uninstaller
	on error
		display alert "nebula-gui isn't installed" message "The uninstaller couldn't find " & uninstaller & "." as warning
		return
	end try

	-- Cancel ends the script quietly (error -128).
	set intro to "This removes the app and its background service, which ends the Nebula connection."
	set choice to "Remove everything also deletes the imported config, the service's logs and every user's saved peer names, app log and settings. Keep settings to use them again after a reinstall."
	set answer to display alert "Uninstall nebula-gui?" message (intro & return & return & choice) buttons {"Cancel", keepButton, everythingButton} default button keepButton cancel button "Cancel"

	set command to quoted form of uninstaller
	if button returned of answer is everythingButton then set command to command & " --remove-everything"
	try
		do shell script command with administrator privileges
	on error errText number errNum
		if errNum is -128 then return -- the password prompt was cancelled
		display alert "nebula-gui couldn't be removed" message errText as critical
		return
	end try
	display alert "nebula-gui has been removed."
end run
