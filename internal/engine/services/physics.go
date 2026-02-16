package services

import (
	"fmt"
	"log"
	"math"

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

// PhysicsEngine handles the simulation of movement, combat, and environment rules.
type PhysicsEngine struct{}

func NewPhysicsEngine() *PhysicsEngine {
	return &PhysicsEngine{}
}

// UpdatePhysics simulates one tick of the world
func (pe *PhysicsEngine) Update(state *pb.WorldState, arenaConfig *pb.ArenaConfig, intents map[string]*pb.BotIntent) *pb.WorldState {
	newState := &pb.WorldState{
		Tick:    state.Tick + 1,
		Bots:    make([]*pb.BotState, 0, len(state.Bots)),
		Bullets: make([]*pb.BulletState, 0),
		Events:  make([]*pb.SimulationEvent, 0),
	}

	// 1. Update Zone (Shrink)
	if state.Zone != nil {
		newState.Zone = &pb.ZoneState{
			X:      state.Zone.X,
			Y:      state.Zone.Y,
			Radius: state.Zone.Radius - 0.05, // Shrink rate
		}
		if newState.Zone.Radius < 50.0 {
			newState.Zone.Radius = 50.0 // Min radius
		}
	}

	if newState.Tick%60 == 0 {
		for _, b := range state.Bots {
			log.Printf("PHYSICS: Bot %s at (%.1f, %.1f), Hull: %.1f Energy: %.1f",
				b.Id, b.Position.X, b.Position.Y, b.Hull, b.Energy)
		}
	}

	// 2. Update Robots & Handle Firing & Zone Damage
	activeRobots := make([]*pb.BotState, 0, len(state.Bots))
	for _, robot := range state.Bots {
		intent := intents[robot.Id]
		updatedRobot := pe.updateRobot(robot, arenaConfig, intent)

		// Zone Damage
		if newState.Zone != nil {
			dx := updatedRobot.Position.X - newState.Zone.X
			dy := updatedRobot.Position.Y - newState.Zone.Y
			dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

			if dist > newState.Zone.Radius {
				pe.applyDamage(updatedRobot, 0.5, newState)
			}
		}

		// Handle Firing
		if intent != nil && intent.FirePower > 0 && updatedRobot.Heat <= 0 && updatedRobot.Energy >= intent.FirePower {
			bulletID := fmt.Sprintf("bullet_%s_%d", robot.Id, newState.Tick)
			newState.Bullets = append(newState.Bullets, &pb.BulletState{
				Id:       bulletID,
				OwnerId:  robot.Id,
				Position: &pb.Vector3{X: updatedRobot.Position.X, Y: updatedRobot.Position.Y, Z: 0},
				Heading:  updatedRobot.GunHeading,
				Velocity: 20.0, // Reduced from 40 to ensure hit detection
				Power:    intent.FirePower,
			})
			updatedRobot.Heat += 1.0 + (intent.FirePower / 5.0)
			updatedRobot.Energy -= intent.FirePower
		}

		if updatedRobot.Heat > 0 {
			updatedRobot.Heat -= 0.1
			if updatedRobot.Heat < 0 {
				updatedRobot.Heat = 0
			}
		}

		// Robot Destruction Check
		if updatedRobot.Hull <= 0 {
			pe.processDeath(updatedRobot, newState)
			continue
		}

		newState.Bots = append(newState.Bots, updatedRobot)

		newState.Bots = append(newState.Bots, updatedRobot)
		activeRobots = append(activeRobots, updatedRobot)
	}

	// 3. Update Bullets & Collision Detection (Bullet vs Robot)
	finalBullets := make([]*pb.BulletState, 0)
	for _, bullet := range state.Bullets {
		newX := bullet.Position.X + float32(math.Sin(float64(bullet.Heading*math.Pi/180.0)))*bullet.Velocity
		newY := bullet.Position.Y - float32(math.Cos(float64(bullet.Heading*math.Pi/180.0)))*bullet.Velocity

		hit := false
		for _, robot := range newState.Bots {
			if robot.Id == bullet.OwnerId {
				continue // No self-harm
			}

			dx := newX - robot.Position.X
			dy := newY - robot.Position.Y
			dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

			if dist < RobotRadius {
				// HIT!
				log.Printf("HIT! Bullet %s hit Robot %s at (%.1f, %.1f). Dist: %.2f", bullet.Id, robot.Id, robot.Position.X, robot.Position.Y, dist)
				pe.applyDamage(robot, bullet.Power, newState)
				newState.Events = append(newState.Events, &pb.SimulationEvent{
					Tick: newState.Tick,
					Event: &pb.SimulationEvent_HitByBullet{
						HitByBullet: &pb.HitByBulletEvent{
							VictimId: robot.Id,
							BulletId: bullet.Id,
							Damage:   bullet.Power,
						},
					},
				})

				// Immediate Death Check after Bullet Hit
				if robot.Hull <= 0 {
					pe.processDeath(robot, newState)
				}

				hit = true
				break
			}
		}

		// Re-filter bots to remove dead ones after bullet hits
		if hit {
			aliveBots := make([]*pb.BotState, 0)
			for _, b := range newState.Bots {
				if b.Hull > 0 {
					aliveBots = append(aliveBots, b)
				}
			}
			newState.Bots = aliveBots
		}

		if !hit && newX >= 0 && newX <= arenaConfig.Width && newY >= 0 && newY <= arenaConfig.Height {
			finalBullets = append(finalBullets, &pb.BulletState{
				Id:       bullet.Id,
				OwnerId:  bullet.OwnerId,
				Position: &pb.Vector3{X: newX, Y: newY, Z: 0},
				Heading:  bullet.Heading,
				Velocity: bullet.Velocity,
				Power:    bullet.Power,
			})
		}
	}
	newState.Bullets = append(newState.Bullets, finalBullets...)

	// 4. Robot-Robot Collision Resolution
	for i := 0; i < len(activeRobots); i++ {
		for j := i + 1; j < len(activeRobots); j++ {
			r1 := activeRobots[i]
			r2 := activeRobots[j]

			dx := float64(r1.Position.X - r2.Position.X)
			dy := float64(r1.Position.Y - r2.Position.Y)
			dist := math.Sqrt(dx*dx + dy*dy)
			minDist := float64(RobotRadius * 2)

			if dist < minDist {
				overlap := minDist - dist
				angle := math.Atan2(dy, dx)
				push := overlap / 2.0

				r1.Position.X += float32(math.Cos(angle) * push)
				r1.Position.Y += float32(math.Sin(angle) * push)
				r2.Position.X -= float32(math.Cos(angle) * push)
				r2.Position.Y -= float32(math.Sin(angle) * push)

				r1.Velocity = 0
				r2.Velocity = 0

				damage := float32(0.6)
				pe.applyDamage(r1, damage, newState)
				pe.applyDamage(r2, damage, newState)
			}
		}
	}

	// 5. Radar Scanning Logic
	for _, scanner := range activeRobots {
		scannerFOV := float32(TankRadarFOV)
		switch scanner.Class {
		case "Scout":
			scannerFOV = ScoutRadarFOV
		case "Sniper":
			scannerFOV = SniperRadarFOV
		}

		for _, target := range activeRobots {
			if scanner.Id == target.Id {
				continue
			}

			dx := target.Position.X - scanner.Position.X
			dy := target.Position.Y - scanner.Position.Y
			dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

			if dist > RadarRange {
				continue
			}

			// Stealth integration: Stealth reduces visibility
			effectiveRange := float32(RadarRange)
			if target.IsStealthed {
				effectiveRange = RadarRange * 0.4
				if dist > effectiveRange {
					continue
				}
			}

			// Calculate angle to target
			angleToTarget := math.Atan2(float64(target.Position.X-scanner.Position.X), float64(scanner.Position.Y-target.Position.Y))
			angleToTargetDeg := float32(angleToTarget * 180.0 / math.Pi)
			if angleToTargetDeg < 0 {
				angleToTargetDeg += 360
			}

			// Compare with RadarHeading
			diff := float32(math.Abs(float64(angleToTargetDeg - scanner.RadarHeading)))
			if diff > 180 {
				diff = 360 - diff
			}

			if diff <= scannerFOV/2.0 {
				// RobotScanned event seems missing in new proto, or handled differently.
				// For now, if we need it, we should add ZoneEntered or similar.
				// But looking at proto, there is no generic RobotScanned.
				// We'll skip adding it for now to fix build, or use a custom event if needed.
				// LEGACY:
				// newState.Events = append(newState.Events, &pb.BattleEvent{
				// 	Type:    "ROBOT_SCANNED",
				// 	BotId:   scanner.Id,
				// 	OtherId: target.Id,
				// 	Tick:    newState.Tick,
				// })
			}
		}
	}

	return newState
}

func (pe *PhysicsEngine) applyDamage(robot *pb.BotState, damage float32, state *pb.WorldState) {
	if robot.ShieldHp > 0 {
		if robot.ShieldHp >= damage {
			robot.ShieldHp -= damage
			damage = 0
		} else {
			damage -= robot.ShieldHp
			robot.ShieldHp = 0
		}
	}

	if damage > 0 {
		robot.Hull = float32(math.Max(0, float64(robot.Hull)-float64(damage)))
	}
}

func (pe *PhysicsEngine) updateRobot(robot *pb.BotState, arena *pb.ArenaConfig, intent *pb.BotIntent) *pb.BotState {
	newRobot := &pb.BotState{
		Id:           robot.Id,
		Name:         robot.Name,
		TeamId:       robot.TeamId,
		Class:        robot.Class,
		Hull:         robot.Hull,
		Energy:       robot.Energy,
		ShieldHp:     robot.ShieldHp,
		Position:     &pb.Vector3{X: robot.Position.X, Y: robot.Position.Y, Z: 0},
		Velocity:     robot.Velocity,
		Heading:      robot.Heading,
		GunHeading:   robot.GunHeading,
		RadarHeading: robot.RadarHeading,
		Heat:         robot.Heat,
		IsStealthed:  robot.IsStealthed,
		Cooldowns:    make(map[string]int32),
	}

	// Copy and Update Cooldowns / Durations
	for k, v := range robot.Cooldowns {
		if v > 0 {
			newRobot.Cooldowns[k] = v - 1
		}
	}

	// Constants based on Class
	maxVel := float32(TankMaxVelocity)
	accel := float32(TankAccel)
	decel := float32(TankDecel)
	maxShield := float32(TankMaxShield)
	maxEnergy := float32(TankMaxEnergy)
	regen := float32(0.2) // Default Tank regen

	switch robot.Class {
	case "Scout":
		maxVel, accel, decel, maxShield, maxEnergy = ScoutMaxVelocity, ScoutAccel, ScoutDecel, ScoutMaxShield, ScoutMaxEnergy
		regen = 0.5
	case "Sniper":
		maxVel, accel, decel, maxShield, maxEnergy = SniperMaxVelocity, SniperAccel, SniperDecel, SniperMaxShield, SniperMaxEnergy
		regen = 0.3
	}

	// Energy Regeneration
	newRobot.Energy += regen
	if newRobot.Energy > maxEnergy {
		newRobot.Energy = maxEnergy
	}
	if newRobot.Energy < 0 {
		newRobot.Energy = 0
	}

	// Handle Special Powers Activation
	if intent != nil && intent.UsePower != pb.PowerType_POWER_NONE {
		switch intent.UsePower {
		case pb.PowerType_SHIELD:
			if newRobot.Energy >= 30 && newRobot.Cooldowns["shield"] <= 0 {
				log.Printf("POWER: Bot %s activating SHIELD", newRobot.Id)
				newRobot.Energy -= 30
				newRobot.ShieldHp = maxShield
				newRobot.Cooldowns["shield"] = 200
			}
		case pb.PowerType_OVERCLOCK:
			if newRobot.Energy >= 40 && newRobot.Cooldowns["overclock"] <= 0 {
				log.Printf("POWER: Bot %s activating OVERCLOCK", newRobot.Id)
				newRobot.Energy -= 40
				newRobot.Cooldowns["overclock_duration"] = 100
				newRobot.Cooldowns["overclock"] = 300
			}
		case pb.PowerType_STEALTH:
			if newRobot.Energy >= 50 && newRobot.Cooldowns["stealth"] <= 0 {
				log.Printf("POWER: Bot %s activating STEALTH", newRobot.Id)
				newRobot.Energy -= 50
				newRobot.Cooldowns["stealth_duration"] = 150
				newRobot.Cooldowns["stealth"] = 400
			}
		}
	}

	// Apply Passive Power Effects
	if newRobot.Cooldowns["overclock_duration"] > 0 {
		maxVel *= 1.5
		accel *= 2.0
	}
	if newRobot.Cooldowns["stealth_duration"] > 0 {
		newRobot.IsStealthed = true
	} else {
		newRobot.IsStealthed = false
	}

	// Apply Movement Intent
	targetVelocity := float32(0.0)
	if intent != nil {
		newRobot.Heading += intent.TurnDegrees
		newRobot.GunHeading += intent.GunTurnDegrees
		newRobot.RadarHeading += intent.RadarTurnDegrees

		// Normalize Angles
		newRobot.Heading = pe.normalizeAngle(newRobot.Heading)
		newRobot.GunHeading = pe.normalizeAngle(newRobot.GunHeading)
		newRobot.RadarHeading = pe.normalizeAngle(newRobot.RadarHeading)

		if intent.MoveDistance > 0 {
			targetVelocity = maxVel
		} else if intent.MoveDistance < 0 {
			targetVelocity = -maxVel
		}
	}

	// Acceleration Physics
	if newRobot.Velocity < targetVelocity {
		newRobot.Velocity += accel
		if newRobot.Velocity > targetVelocity {
			newRobot.Velocity = targetVelocity
		}
	} else if newRobot.Velocity > targetVelocity {
		newRobot.Velocity -= decel
		if newRobot.Velocity < targetVelocity {
			newRobot.Velocity = targetVelocity
		}
	}

	// Movement
	rad := float64(newRobot.Heading) * (math.Pi / 180.0)
	newX := newRobot.Position.X + float32(math.Sin(rad))*newRobot.Velocity
	newY := newRobot.Position.Y - float32(math.Cos(rad))*newRobot.Velocity

	// Wall Collision
	margin := float32(RobotRadius)
	if newX <= margin || newX >= float32(arena.Width)-margin || newY <= margin || newY >= float32(arena.Height)-margin {
		newRobot.Velocity = 0
	}

	newRobot.Position.X = float32(math.Max(float64(margin), math.Min(float64(arena.Width)-float64(margin), float64(newX))))
	newRobot.Position.Y = float32(math.Max(float64(margin), math.Min(float64(arena.Height)-float64(margin), float64(newY))))

	return newRobot
}

func (pe *PhysicsEngine) processDeath(robot *pb.BotState, newState *pb.WorldState) {
	log.Printf("DEATH: Robot %s destroyed at Tick %d", robot.Id, newState.Tick)
	newState.Events = append(newState.Events, &pb.SimulationEvent{
		Tick: newState.Tick,
		Event: &pb.SimulationEvent_Death{
			Death: &pb.DeathEvent{
				BotId: robot.Id,
			},
		},
	})
}

func (pe *PhysicsEngine) normalizeAngle(angle float32) float32 {
	angle = float32(math.Mod(float64(angle), 360.0))
	if angle < 0 {
		angle += 360
	}
	return angle
}
