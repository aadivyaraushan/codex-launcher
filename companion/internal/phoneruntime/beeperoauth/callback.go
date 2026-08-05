package beeperoauth

import "errors"

type Port9445 struct {
	open     bool
	consumed bool
}

func NewPending() *Port9445 { return &Port9445{open: true} }

var (
	ErrClosed   = errors.New("beeper oauth: callback port closed")
	ErrConsumed = errors.New("beeper oauth: second callback rejected")
)

func (p *Port9445) Accept(code string) error {
	if p == nil || !p.open {
		return ErrClosed
	}
	if p.consumed {
		return ErrConsumed
	}
	if code == "" {
		p.open = false
		return errors.New("beeper oauth: empty code")
	}
	p.consumed = true
	p.open = false
	return nil
}

func (p *Port9445) Close() { p.open = false; }

func (p *Port9445) Open() bool { return p != nil && p.open }
