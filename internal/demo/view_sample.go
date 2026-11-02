package demo

import (
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

func captureView(pl *common.Player) *match.ViewSample {
	if pl == nil {
		return nil
	}
	pawn := pl.PlayerPawnEntity()
	if pawn == nil || pawn.Property("m_angEyeAngles") == nil {
		return nil
	}
	v := &match.ViewSample{Yaw: float64(pl.ViewDirectionX()), Pitch: float64(pl.ViewDirectionY())}
	if eye, ok := pl.PositionEyes(); ok {
		v.Eye = &match.Vec{X: eye.X, Y: eye.Y, Z: eye.Z}
	}
	return v
}
