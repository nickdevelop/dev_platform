@echo off
rem Windows mock for the end-to-end workbench test: the platform resolves
rem opencode.cmd from PATH (the extensionless mock is invisible to Windows
rem lookup) and runs the Node mock in the same directory with it.
node "%~dp0opencode" %*
