//go:build windows

package windows

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The platform keeps one Job Object for all children it starts, with
// KILL_ON_JOB_CLOSE: when the last platform handle closes — normal exit or
// crash — the OS ends every member, so no child is left behind.
//
// Assignment happens in Attach, right after the child starts. A child that
// exits within that microscopic window can escape; the platform's children
// (agent CLIs, service servers) live far longer, so the window is accepted
// and documented instead of suspending every child at creation.
var (
	jobOnce   sync.Once
	jobHandle windows.Handle
	jobErr    error
)

// platformJob returns the platform Job Object, creating it once.
func platformJob() (windows.Handle, error) {
	jobOnce.Do(func() {
		handle, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			jobErr = fmt.Errorf("platform: create job object: %w", err)
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
			BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
				LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
			},
		}
		_, err = windows.SetInformationJobObject(
			handle,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
		)
		if err != nil {
			windows.CloseHandle(handle)
			jobErr = fmt.Errorf("platform: configure job object: %w", err)
			return
		}
		jobHandle = handle
	})
	return jobHandle, jobErr
}

// assignToPlatformJob enrolls the process handle in the platform Job Object.
func assignToPlatformJob(handle windows.Handle) error {
	job, err := platformJob()
	if err != nil {
		return err
	}
	if err := windows.AssignProcessToJobObject(job, handle); err != nil {
		return fmt.Errorf("platform: assign child process to job: %w", err)
	}
	return nil
}
