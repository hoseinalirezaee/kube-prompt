package prompt

import "sync"

type completionRequest struct {
	seq int64
	doc Document
}

type completionResult struct {
	seq         int64
	suggestions []Suggest
}

type asyncCompletion struct {
	completer Completer
	requests  chan completionRequest
	results   chan completionResult
	stopCh    chan struct{}
	stopOnce  sync.Once
}

func newAsyncCompletion(completer Completer) *asyncCompletion {
	a := &asyncCompletion{
		completer: completer,
		requests:  make(chan completionRequest, 1),
		results:   make(chan completionResult, 1),
		stopCh:    make(chan struct{}),
	}
	go a.run()
	return a
}

func (a *asyncCompletion) Request(req completionRequest) {
	if a == nil {
		return
	}
	select {
	case a.requests <- req:
		return
	case <-a.stopCh:
		return
	default:
	}

	select {
	case <-a.requests:
	default:
	}

	select {
	case a.requests <- req:
	case <-a.stopCh:
	}
}

func (a *asyncCompletion) Results() <-chan completionResult {
	if a == nil {
		return nil
	}
	return a.results
}

func (a *asyncCompletion) Stop() {
	if a == nil {
		return
	}
	a.stopOnce.Do(func() {
		close(a.stopCh)
	})
}

func (a *asyncCompletion) run() {
	for {
		select {
		case <-a.stopCh:
			return
		case req := <-a.requests:
			req = a.latestRequest(req)
			suggestions := a.completer(req.doc)
			a.sendResult(completionResult{seq: req.seq, suggestions: suggestions})
		}
	}
}

func (a *asyncCompletion) latestRequest(req completionRequest) completionRequest {
	for {
		select {
		case newer := <-a.requests:
			req = newer
		default:
			return req
		}
	}
}

func (a *asyncCompletion) sendResult(result completionResult) {
	select {
	case a.results <- result:
		return
	case <-a.stopCh:
		return
	default:
	}

	select {
	case <-a.results:
	default:
	}

	select {
	case a.results <- result:
	case <-a.stopCh:
	}
}

type completionState struct {
	worker    *asyncCompletion
	latestSeq int64
}

func newCompletionState(completer Completer) *completionState {
	return &completionState{worker: newAsyncCompletion(completer)}
}

func (s *completionState) Request(doc Document) {
	if s == nil {
		return
	}
	s.latestSeq++
	s.worker.Request(completionRequest{seq: s.latestSeq, doc: doc})
}

func (s *completionState) Invalidate() {
	if s == nil {
		return
	}
	s.latestSeq++
}

func (s *completionState) Results() <-chan completionResult {
	if s == nil {
		return nil
	}
	return s.worker.Results()
}

func (s *completionState) Apply(manager *CompletionManager, result completionResult) bool {
	if s == nil || result.seq != s.latestSeq {
		return false
	}
	manager.SetSuggestions(result.suggestions)
	return true
}

func (s *completionState) Stop() {
	if s == nil {
		return
	}
	s.worker.Stop()
}
