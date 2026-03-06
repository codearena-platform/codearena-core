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

	// 0. Initialize Quadtree for optimizations
	qtBoundary := Rectangle{X: arenaConfig.Width / 2, Y: arenaConfig.Height / 2, W: arenaConfig.Width / 2, H: arenaConfig.Height / 2}
	qt := NewQuadtree(qtBoundary, 4)
	for _, b := range state.Bots {
		qt.Insert(b)
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

	// 2. Update Robots & Handle Firing & Zone Damage
	activeRobots := make([]*pb.BotState, 0, len(state.Bots))
	for _, robot := range state.Bots {
		intent := intents[robot.Id]
		updatedRobot := pe.updateRobot(robot, arenaConfig, intent)

		// Zone Damage (Shrink)
		if newState.Zone != nil {
			dx := updatedRobot.Position.X - newState.Zone.X
			dy := updatedRobot.Position.Y - newState.Zone.Y
			dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

			if dist > newState.Zone.Radius {
				pe.applyDamage(updatedRobot, 0.5, newState)
			}
		}

		// Arena Zones (Functional)
		for _, zone := range arenaConfig.Zones {
			dx := updatedRobot.Position.X - zone.Position.X
			dy := updatedRobot.Position.Y - zone.Position.Y
			dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

			if dist <= zone.Radius {
				switch zone.Type {
				case "HEAL":
					updatedRobot.Hull = float32(math.Min(100.0, float64(updatedRobot.Hull+HealZoneAmount)))
				case "ENERGY":
					updatedRobot.Energy += EnergyZoneAmount
				case "HAZARD":
					pe.applyDamage(updatedRobot, HazardZoneDamage, newState)
				}

				newState.Events = append(newState.Events, &pb.SimulationEvent{
					Tick: newState.Tick,
					Event: &pb.SimulationEvent_ZoneEntered{
						ZoneEntered: &pb.ZoneEnteredEvent{
							BotId:  updatedRobot.Id,
							ZoneId: zone.Id,
							Type:   zone.Type,
						},
					},
				})
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
				Velocity: 20.0,
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

		if updatedRobot.Hull <= 0 {
			pe.processDeath(updatedRobot, newState)
			continue
		}

		newState.Bots = append(newState.Bots, updatedRobot)
		activeRobots = append(activeRobots, updatedRobot)
	}

	// 3. Update Bullets & Collision Detection (Bullet vs Robot via Quadtree)
	finalBullets := make([]*pb.BulletState, 0)
	for _, bullet := range state.Bullets {
		newX := bullet.Position.X + float32(math.Sin(float64(bullet.Heading*math.Pi/180.0)))*bullet.Velocity
		newY := bullet.Position.Y - float32(math.Cos(float64(bullet.Heading*math.Pi/180.0)))*bullet.Velocity

		hit := false
		bulletRange := Rectangle{X: newX, Y: newY, W: RobotRadius, H: RobotRadius}
		var nearbyBots []*pb.BotState
		qt.Query(bulletRange, &nearbyBots)

		for _, robot := range nearbyBots {
			if robot.Id == bullet.OwnerId {
				continue
			}

			dx := newX - robot.Position.X
			dy := newY - robot.Position.Y
			dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

			if dist < RobotRadius {
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

				if robot.Hull <= 0 {
					pe.processDeath(robot, newState)
				}
				hit = true
				break
			}
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

	// 4. Robot-Robot Collision Resolution via Quadtree
	for _, r1 := range activeRobots {
		collisionRange := Rectangle{X: r1.Position.X, Y: r1.Position.Y, W: RobotRadius * 2, H: RobotRadius * 2}
		var nearbyBots []*pb.BotState
		qt.Query(collisionRange, &nearbyBots)

		for _, r2 := range nearbyBots {
			if r1.Id == r2.Id {
				continue
			}

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

	// 5. Radar Scanning Logic via Quadtree
	for _, scanner := range activeRobots {
		scannerFOV := float32(TankRadarFOV)
		if scanner.Class == "Scout" {
			scannerFOV = ScoutRadarFOV
		} else if scanner.Class == "Sniper" {
			scannerFOV = SniperRadarFOV
		}

		radarQueryRange := Rectangle{X: scanner.Position.X, Y: scanner.Position.Y, W: RadarRange, H: RadarRange}
		var nearbyBots []*pb.BotState
		qt.Query(radarQueryRange, &nearbyBots)

		for _, target := range nearbyBots {
			if scanner.Id == target.Id {
				continue
			}

			dx := target.Position.X - scanner.Position.X
			dy := target.Position.Y - scanner.Position.Y
			dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

			if dist > RadarRange {
				continue
			}

			effectiveRange := float32(RadarRange)
			if target.IsStealthed {
				effectiveRange = RadarRange * 0.4
				if dist > effectiveRange {
					continue
				}
			}

			angleToTarget := math.Atan2(float64(target.Position.X-scanner.Position.X), float64(scanner.Position.Y-target.Position.Y))
			angleToTargetDeg := float32(angleToTarget * 180.0 / math.Pi)
			if angleToTargetDeg < 0 {
				angleToTargetDeg += 360
			}

			diff := float32(math.Abs(float64(angleToTargetDeg - scanner.RadarHeading)))
			if diff > 180 {
				diff = 360 - diff
			}

			if diff <= scannerFOV/2.0 {
				// Event omitted per current proto limitations in existing code
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

// FilterStateForBot creates a customized WorldState for a specific bot based on its sensors
func (pe *PhysicsEngine) FilterStateForBot(botID string, fullState *pb.WorldState) *pb.WorldState {
	var viewer *pb.BotState
	for _, b := range fullState.Bots {
		if b.Id == botID {
			viewer = b
			break
		}
	}

	// If viewer not found (dead?), they see nothing or full state?
	// Usually they see nothing if they are dead.
	if viewer == nil {
		return &pb.WorldState{
			Tick:   fullState.Tick,
			Status: fullState.Status,
			Zone:   fullState.Zone,
		}
	}

	filteredState := &pb.WorldState{
		Tick:   fullState.Tick,
		Status: fullState.Status,
		Zone:   fullState.Zone,
		Events: make([]*pb.SimulationEvent, 0),
		Bots:   make([]*pb.BotState, 0),
	}

	// 1. Bots are always visible to themselves
	filteredState.Bots = append(filteredState.Bots, viewer)

	// 2. Filter other bots
	for _, target := range fullState.Bots {
		if target.Id == botID {
			continue
		}

		dx := target.Position.X - viewer.Position.X
		dy := target.Position.Y - viewer.Position.Y
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

		// Logic:
		// - Close range: Always visible (Sensors)
		// - Radar range: Visible if within FOV or Scanned
		// - Stealth: Harder to see

		visible := false
		if dist < 150.0 { // Proximity Sensor
			visible = true
		} else if dist < RadarRange {
			// Basic Radar Logic: Check FOV (Legacy behavior but per-bot now)
			angleToTarget := math.Atan2(float64(target.Position.X-viewer.Position.X), float64(viewer.Position.Y-target.Position.Y))
			angleToTargetDeg := float32(angleToTarget * 180.0 / math.Pi)
			if angleToTargetDeg < 0 {
				angleToTargetDeg += 360
			}

			diff := float32(math.Abs(float64(angleToTargetDeg - viewer.RadarHeading)))
			if diff > 180 {
				diff = 360 - diff
			}

			radarFOV := float32(TankRadarFOV)
			if viewer.Class == "Scout" {
				radarFOV = ScoutRadarFOV
			} else if viewer.Class == "Sniper" {
				radarFOV = SniperRadarFOV
			}

			if diff <= radarFOV/2.0 {
				visible = true
			}
		}

		if target.IsStealthed && dist > 100.0 {
			visible = false // Stealth works unless very close
		}

		if visible {
			filteredState.Bots = append(filteredState.Bots, target)
		}
	}

	// 3. Filter Events (only relevant to the bot)
	for _, ev := range fullState.Events {
		relevant := false
		// Broad relevance: Death of any bot is public?
		// For now, let's say only events involving the viewer id are relevant
		// OR events at visible locations.

		if death := ev.GetDeath(); death != nil {
			relevant = true // Deaths are public news
		} else if hit := ev.GetHitByBullet(); hit != nil {
			if hit.VictimId == botID {
				relevant = true
			}
		} else if zone := ev.GetZoneEntered(); zone != nil {
			if zone.BotId == botID {
				relevant = true
			}
		}

		if relevant {
			filteredState.Events = append(filteredState.Events, ev)
		}
	}

	return filteredState
}

// --- Quadtree Implementation for Spatial Partitioning ---

type Rectangle struct {
	X, Y, W, H float32
}

func (r Rectangle) Contains(b *pb.BotState) bool {
	return b.Position.X >= r.X-r.W && b.Position.X <= r.X+r.W &&
		b.Position.Y >= r.Y-r.H && b.Position.Y <= r.Y+r.H
}

func (r Rectangle) Intersects(other Rectangle) bool {
	return !(other.X-other.W > r.X+r.W ||
		other.X+other.W < r.X-r.W ||
		other.Y-other.H > r.Y+r.H ||
		other.Y+other.H < r.Y-r.H)
}

type Quadtree struct {
	Boundary       Rectangle
	Capacity       int
	Bots           []*pb.BotState
	Divided        bool
	NW, NE, SW, SE *Quadtree
}

func NewQuadtree(boundary Rectangle, capacity int) *Quadtree {
	return &Quadtree{
		Boundary: boundary,
		Capacity: capacity,
		Bots:     make([]*pb.BotState, 0),
		Divided:  false,
	}
}

func (qt *Quadtree) Subdivide() {
	x, y, w, h := qt.Boundary.X, qt.Boundary.Y, qt.Boundary.W/2, qt.Boundary.H/2
	qt.NW = NewQuadtree(Rectangle{x - w, y - h, w, h}, qt.Capacity)
	qt.NE = NewQuadtree(Rectangle{x + w, y - h, w, h}, qt.Capacity)
	qt.SW = NewQuadtree(Rectangle{x - w, y + h, w, h}, qt.Capacity)
	qt.SE = NewQuadtree(Rectangle{x + w, y + h, w, h}, qt.Capacity)
	qt.Divided = true
}

func (qt *Quadtree) Insert(bot *pb.BotState) bool {
	if !qt.Boundary.Contains(bot) {
		return false
	}

	if !qt.Divided {
		if len(qt.Bots) < qt.Capacity {
			qt.Bots = append(qt.Bots, bot)
			return true
		}
		qt.Subdivide()
		for _, b := range qt.Bots {
			qt.insertToChildren(b)
		}
		qt.Bots = nil
	}

	return qt.insertToChildren(bot)
}

func (qt *Quadtree) insertToChildren(bot *pb.BotState) bool {
	if qt.NW.Insert(bot) {
		return true
	}
	if qt.NE.Insert(bot) {
		return true
	}
	if qt.SW.Insert(bot) {
		return true
	}
	if qt.SE.Insert(bot) {
		return true
	}
	return false
}

func (qt *Quadtree) Query(rangeRect Rectangle, found *[]*pb.BotState) {
	if !qt.Boundary.Intersects(rangeRect) {
		return
	}

	if qt.Divided {
		qt.NW.Query(rangeRect, found)
		qt.NE.Query(rangeRect, found)
		qt.SW.Query(rangeRect, found)
		qt.SE.Query(rangeRect, found)
		return
	}

	for _, b := range qt.Bots {
		if rangeRect.Contains(b) {
			*found = append(*found, b)
		}
	}
}
