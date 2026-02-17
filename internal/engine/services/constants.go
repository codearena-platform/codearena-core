package services

// Physics Constants
const (
	// Tank Class Constants
	TankMaxVelocity = 6.0
	TankAccel       = 0.5
	TankDecel       = 1.0
	TankGunCooling  = 0.1
	TankMaxShield   = 50.0
	TankMaxEnergy   = 150.0
	TankRadarFOV    = 60.0

	// Scout Class Constants
	ScoutMaxVelocity = 12.0
	ScoutAccel       = 2.0
	ScoutDecel       = 3.0
	ScoutGunCooling  = 0.2
	ScoutMaxShield   = 20.0
	ScoutMaxEnergy   = 80.0
	ScoutRadarFOV    = 120.0

	// Sniper Class Constants
	SniperMaxVelocity = 8.0
	SniperAccel       = 1.0
	SniperDecel       = 2.0
	SniperGunCooling  = 0.05
	SniperMaxShield   = 30.0
	SniperMaxEnergy   = 100.0
	SniperRadarFOV    = 30.0

	// Global Constants
	RadarRange  = 800.0
	RobotRadius = 20.0 // Effective radius for collision/hit detection

	// Zone Effects (per tick)
	HealZoneAmount   = 0.1
	EnergyZoneAmount = 0.5
	HazardZoneDamage = 0.2
)
