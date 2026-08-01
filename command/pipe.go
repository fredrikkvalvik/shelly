package command

import (
	"io"
	"strings"
	"sync"
)

// PipeError reports which stages of a pipeline failed.
//
// A shell takes a pipeline's exit status from its last stage only, so
// `ls missing | wc -l` succeeds. Last exposes that stage's error for callers
// who want the same behaviour, while Unwrap keeps errors.Is and errors.As
// working across every stage.
type PipeError struct {
	Errs []error // one entry per stage, nil where the stage succeeded
}

func (e *PipeError) Error() string {
	msgs := make([]string, 0, len(e.Errs))
	for _, err := range e.Errs {
		if err != nil {
			msgs = append(msgs, err.Error())
		}
	}
	return strings.Join(msgs, "\n")
}

func (e *PipeError) Unwrap() []error { return e.Errs }

// the last stage's error, which is the pipeline's status in a shell
func (e *PipeError) Last() error {
	if len(e.Errs) == 0 {
		return nil
	}
	return e.Errs[len(e.Errs)-1]
}

func Pipe(cmd *Command, rest ...*Command) error {
	cmds := append([]*Command{cmd}, rest...)

	// each cmd owns the writer feeding the next, and the reader feeding itself
	ws := make([]*io.PipeWriter, len(cmds))
	rs := make([]*io.PipeReader, len(cmds))
	for i := range len(cmds) - 1 {
		r, w := io.Pipe()
		cmds[i].cfg.stdout = w
		cmds[i+1].cfg.stdin = r
		ws[i], rs[i+1] = w, r
	}

	var (
		wg   sync.WaitGroup
		errs = make([]error, len(cmds))
	)
	for i, c := range cmds {
		wg.Go(func() {
			errs[i] = c.Run()
			if ws[i] != nil {
				ws[i].Close() // EOF for downstream
			}
			if rs[i] != nil {
				rs[i].Close() // unblock upstream write if we exited early
			}
		})
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return &PipeError{Errs: errs}
		}
	}
	return nil
}
