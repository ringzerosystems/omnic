package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
)

// Player orchestrates non-blocking audio playback using ffplay.
type Player struct {
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	mu      sync.Mutex
	playing bool
}

// NewPlayer constructs a new Player.
func NewPlayer() *Player {
	return &Player{}
}

// Play begins playing an audio file or stream URL in the background.
// Any currently active playback is automatically stopped.
func (p *Player) Play(ctx context.Context, mediaPathOrURL string) (<-chan error, error) {
	if mediaPathOrURL == "" {
		return nil, errors.New("empty media path")
	}

	_ = p.Stop()

	p.mu.Lock()
	defer p.mu.Unlock()

	playCtx, cancel := context.WithCancel(ctx)
	//nolint:gosec // arguments are constructed internally
	cmd := exec.CommandContext(playCtx, "ffplay", "-nodisp", "-autoexit", "-loglevel", "error", mediaPathOrURL)

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("starting ffplay: %w", err)
	}

	p.cmd = cmd
	p.cancel = cancel
	p.playing = true

	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		if p.cmd == cmd {
			p.playing = false
			p.cmd = nil
			p.cancel = nil
		}
		p.mu.Unlock()

		if err != nil && !errors.Is(err, context.Canceled) {
			done <- err
		} else {
			done <- nil
		}
		close(done)
	}()

	return done, nil
}

// Stop terminates any currently active audio playback.
func (p *Player) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.playing {
		return nil
	}

	p.playing = false
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		p.cmd = nil
	}
	return nil
}

// IsPlaying returns true if audio is actively playing.
func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing
}
