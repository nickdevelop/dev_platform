@echo off
rem Windows mock for the end-to-end workbench test: the platform resolves
rem pi.cmd from PATH (the extensionless mock is invisible to Windows lookup)
rem and runs the Node mock in the same directory with it.
node "%~dp0pi" %*
