package session

// Metadata returns a defensive snapshot of session identity and state.
func (s *Session) Metadata() Metadata {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return Metadata{
		ID:       s.id,
		Command:  cloneStrings(s.command),
		Dir:      s.dir,
		State:    s.state,
		ExitCode: s.result.ExitCode,
	}
}

// Done closes after the child exit and all PTY output has been published.
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// Wait blocks until the session is fully drained and returns the child status.
func (s *Session) Wait() (Result, error) {
	<-s.done
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.result, s.runErr
}

func (s *Session) finish(result Result, runErr error) {
	s.stateMu.Lock()
	s.result = result
	s.runErr = runErr
	s.state = StateExited
	s.stateMu.Unlock()
	s.closeSubscribers()
	close(s.done)
}

func isDone(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}
