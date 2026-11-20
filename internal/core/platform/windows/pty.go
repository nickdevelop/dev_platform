//go:build windows

package windows

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ErrPTYUnsupported indicates pseudoterminal is unsupported on this Windows system.
var ErrPTYUnsupported = errors.New("pseudoterminal is unsupported on this Windows version (requires Windows 10 1809+)")

// PTY allocates ConPTY pseudoterminals on Windows.
type PTY struct{}

// Supported reports whether Windows ConPTY is available in kernel32.dll.
func (PTY) Supported() bool {
	mod := windows.NewLazySystemDLL("kernel32.dll")
	return mod.NewProc("CreatePseudoConsole").Find() == nil
}

// PTYSession wraps the Windows pseudo console and child process.
type PTYSession struct {
	hPC       windows.Handle
	hProcess  windows.Handle
	pid       int
	inPipe    *os.File
	outPipe   *os.File
	closeOnce sync.Once
	waitOnce  sync.Once
	waitErr   error
	waitDone  chan struct{}
}

func (s *PTYSession) Read(p []byte) (int, error) {
	if s.outPipe == nil {
		return 0, errors.New("pty is closed")
	}
	return s.outPipe.Read(p)
}

func (s *PTYSession) Write(p []byte) (int, error) {
	if s.inPipe == nil {
		return 0, errors.New("pty is closed")
	}
	return s.inPipe.Write(p)
}

func (s *PTYSession) Resize(rows, cols uint16) error {
	if s.hPC == 0 {
		return errors.New("pty is closed")
	}
	if rows == 0 {
		rows = 24
	}
	if cols == 0 {
		cols = 80
	}
	coord := windows.Coord{
		X: int16(cols),
		Y: int16(rows),
	}
	return windows.ResizePseudoConsole(s.hPC, coord)
}

func (s *PTYSession) Close() error {
	var err error
	s.closeOnce.Do(func() {
		// Close input pipe to signal EOF to child
		if s.inPipe != nil {
			_ = s.inPipe.Close()
			s.inPipe = nil
		}
		// Close pseudo console
		if s.hPC != 0 {
			windows.ClosePseudoConsole(s.hPC)
			s.hPC = 0
		}
		// Close output pipe
		if s.outPipe != nil {
			_ = s.outPipe.Close()
			s.outPipe = nil
		}
		// Terminate process if still running
		if s.hProcess != 0 {
			_ = windows.TerminateProcess(s.hProcess, 1)
			_ = windows.CloseHandle(s.hProcess)
			s.hProcess = 0
		}
		if s.waitDone != nil {
			select {
			case <-s.waitDone:
			case <-time.After(500 * time.Millisecond):
			}
		}
	})
	return err
}

func (s *PTYSession) Wait() error {
	if s.waitDone == nil {
		return nil
	}
	<-s.waitDone
	return s.waitErr
}

func (s *PTYSession) Pid() int {
	return s.pid
}

func (s *PTYSession) reap() {
	s.waitOnce.Do(func() {
		if s.hProcess != 0 {
			_, _ = windows.WaitForSingleObject(s.hProcess, windows.INFINITE)
			var exitCode uint32
			if err := windows.GetExitCodeProcess(s.hProcess, &exitCode); err == nil && exitCode != 0 {
				s.waitErr = fmt.Errorf("exit status %d", exitCode)
			}
		}
		if s.waitDone != nil {
			close(s.waitDone)
		}
	})
}

// Start launches cmd attached to a Windows Pseudo Console (ConPTY).
func (PTY) Start(cmd *exec.Cmd, rows, cols uint16) (*PTYSession, error) {
	if !(PTY{}).Supported() {
		return nil, ErrPTYUnsupported
	}
	if rows == 0 {
		rows = 24
	}
	if cols == 0 {
		cols = 80
	}

	var inRead, inWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &inWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("create in pipe: %w", err)
	}

	var outRead, outWrite windows.Handle
	if err := windows.CreatePipe(&outRead, &outWrite, nil, 0); err != nil {
		_ = windows.CloseHandle(inRead)
		_ = windows.CloseHandle(inWrite)
		return nil, fmt.Errorf("create out pipe: %w", err)
	}

	coord := windows.Coord{
		X: int16(cols),
		Y: int16(rows),
	}
	var hPC windows.Handle
	err := windows.CreatePseudoConsole(coord, inRead, outWrite, 0, &hPC)
	_ = windows.CloseHandle(inRead)
	_ = windows.CloseHandle(outWrite)
	if err != nil {
		_ = windows.CloseHandle(inWrite)
		_ = windows.CloseHandle(outRead)
		return nil, fmt.Errorf("create pseudo console: %w", err)
	}

	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		windows.ClosePseudoConsole(hPC)
		_ = windows.CloseHandle(inWrite)
		_ = windows.CloseHandle(outRead)
		return nil, fmt.Errorf("new proc thread attr list: %w", err)
	}
	defer attrList.Delete()

	if err := attrList.Update(
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		unsafe.Pointer(hPC),
		unsafe.Sizeof(hPC),
	); err != nil {
		windows.ClosePseudoConsole(hPC)
		_ = windows.CloseHandle(inWrite)
		_ = windows.CloseHandle(outRead)
		return nil, fmt.Errorf("update proc thread attr: %w", err)
	}

	siEx := &windows.StartupInfoEx{}
	siEx.Cb = uint32(unsafe.Sizeof(*siEx))
	siEx.Flags = windows.STARTF_USESTDHANDLES
	siEx.ProcThreadAttributeList = attrList.List()

	var cmdLine string
	if len(cmd.Args) > 0 {
		cmdLine = windows.ComposeCommandLine(cmd.Args)
	} else {
		cmdLine = windows.EscapeArg(cmd.Path)
	}
	cmdLinePtr, err := windows.UTF16PtrFromString(cmdLine)
	if err != nil {
		windows.ClosePseudoConsole(hPC)
		_ = windows.CloseHandle(inWrite)
		_ = windows.CloseHandle(outRead)
		return nil, fmt.Errorf("cmdline utf16: %w", err)
	}

	var dirPtr *uint16
	if cmd.Dir != "" {
		dirPtr, err = windows.UTF16PtrFromString(cmd.Dir)
		if err != nil {
			windows.ClosePseudoConsole(hPC)
			_ = windows.CloseHandle(inWrite)
			_ = windows.CloseHandle(outRead)
			return nil, fmt.Errorf("dir utf16: %w", err)
		}
	}

	var envPtr *uint16
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT)
	if len(cmd.Env) > 0 {
		envBlock := createEnvBlock(cmd.Env)
		envPtr = &envBlock[0]
		flags |= windows.CREATE_UNICODE_ENVIRONMENT
	}

	var pi windows.ProcessInformation
	err = windows.CreateProcess(
		nil,
		cmdLinePtr,
		nil,
		nil,
		false,
		flags,
		envPtr,
		dirPtr,
		&siEx.StartupInfo,
		&pi,
	)
	if err != nil {
		windows.ClosePseudoConsole(hPC)
		_ = windows.CloseHandle(inWrite)
		_ = windows.CloseHandle(outRead)
		return nil, fmt.Errorf("create process: %w", err)
	}
	_ = windows.CloseHandle(pi.Thread)

	inFile := os.NewFile(uintptr(inWrite), "conpty-in")
	outFile := os.NewFile(uintptr(outRead), "conpty-out")
	sess := &PTYSession{
		hPC:      hPC,
		hProcess: pi.Process,
		pid:      int(pi.ProcessId),
		inPipe:   inFile,
		outPipe:  outFile,
		waitDone: make(chan struct{}),
	}
	go sess.reap()
	return sess, nil
}

func createEnvBlock(env []string) []uint16 {
	if len(env) == 0 {
		return nil
	}
	var block []uint16
	for _, kv := range env {
		u, _ := windows.UTF16FromString(kv)
		block = append(block, u...)
	}
	block = append(block, 0)
	return block
}
