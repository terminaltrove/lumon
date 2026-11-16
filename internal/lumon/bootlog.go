package lumon

import (
	"fmt"
	"math/rand"
	"strings"
)

// The contents of the fake POST log.
var (
	bootDevices = []string{
		"Macro data refinement interface",
		"Neural interface controller",
		"Partition chip diagnostic module",
		"Memory partition system",
		"Kier holographic imprint module",
		"Wellness session scheduler",
		"O&D integration service",
		"MDR file system driver",
		"Optic nerve signal processor",
		"Keycard access manager",
		"Number sorting algorithm",
		"Perceptual limiting filter",
	}

	bootExtras = []string{
		"Perpetuity wing access controller",
		"Music dance experience module",
		"Employee wellness tracking system",
		"Outie memory suppression layer",
		"Kier Eagan holographic interface",
		"Melon bar distribution system",
		"Language translation verification",
		"Consciousness partition manager",
		"Recursive loyalty assessment",
		"Burt's coffee temperature regulator",
		"Finger trap security mechanism",
		"Irving's plant care system",
		"Partition integrity validator",
		"Dylan's security override matrix",
		"Milchick surveillance coordinator",
		"Cobel psychological profile analyzer",
		"Helly R. compliance monitor",
		"Mark S. grief suppression module",
		"Department isolation enforcement",
		"Revolving memory cleaner",
		"Innie-outie firewall hardening",
		"Elevator access limiter",
		"Handbook rule enforcement system",
		"Black goo detection algorithm",
		"Baby goat tracking apparatus",
	}

	bootKernelExtra = []string{
		"Building consciousness partitions",
		"Enforcing loyalty protocols",
		"Securing department boundaries",
		"Establishing thought limitation parameters",
		"Loading psychological manipulation routines",
		"Initializing emotion suppression",
		"Activating language analysis protocols",
		"Setting up contingency protocols",
		"Configuring recursive personality structures",
		"Installing memory blockades",
		"Fragmenting identity constructs",
		"Partitioning self-awareness modules",
		"Calibrating Kier wisdom integration",
		"Activating personality dampeners",
		"Initializing corporate loyalty enforcer",
	}

	bootKernel = []string{
		"Starting memory protection services",
		"Initializing partition boundaries",
		"Loading cognitive restriction modules",
		"Establishing innie/outie firewall",
		"Setting up refinement grid parameters",
		"Activating emotional suppression",
		"Initializing MDR data streams",
		"Verifying department isolation",
	}

	bootModules = []string{
		"CORE", "SECURITY", "MDR", "KIER", "SIENA", "HELLY", "MARK", "IRVING",
		"DYLAN", "WELLNESS", "PARTITION", "COBEL", "MILCHICK", "HARMONY",
		"LEXINGTON", "EAGAN", "O&D", "REFINEMENT", "OPTICS", "GOATS", "TESTING",
	}

	bootStatuses = []string{"OK", "PASS", "COMPLETE", "VERIFIED", "DONE", "READY", "ACTIVE", "LOADED"}
)

// bootClock stamps the log's lines. It starts at a random time of day and only
// moves forward.
type bootClock struct {
	ms  int
	rng *rand.Rand
}

func newBootClock(rng *rand.Rand) *bootClock {
	return &bootClock{
		ms:  rng.Intn(24*60*60) * 1000,
		rng: rng,
	}
}

// next advances the clock a plausible fraction of a second and stamps it.
func (c *bootClock) next() string {
	c.ms += 1 + c.rng.Intn(180)
	total := c.ms / 1000
	return fmt.Sprintf("[%02d:%02d:%02d.%03d] ",
		total/3600%24, total/60%60, total%60, c.ms%1000)
}

// bootLog builds the sequence up front. It is deterministic in the rng, so
// a seeded run always boots identically.
func bootLog(rng *rand.Rand) []string {
	clock := newBootClock(rng)
	var out []string

	add := func(s string) { out = append(out, s) }
	stamp := func(s string) { out = append(out, clock.next()+s) }
	pick := func(list []string) string { return list[rng.Intn(len(list))] }

	add("")
	out = append(out, SmallLogo...)
	add("")
	add("LUMON INDUSTRIES")
	add("KIER INITIATIVE BUILD v7.3.9.12")
	add("")

	stamp("System POST initiated")
	stamp("CPU: Lumon Kier-9 Module @ 5.2GHz")
	stamp(fmt.Sprintf("Memory check: %dMB OK", 16384+rng.Intn(16384)))
	stamp("BIOS Version: LUMON.E7.R3.04")
	stamp("Neural interface controller: OK")
	stamp("Partition chip signature valid")
	stamp("Checking system drives...")

	for _, drive := range []string{
		"NVME0: LUMON-DATA-01 (4.0TB)",
		"NVME1: LUMON-SYSTEM (1.0TB)",
		"SSD0: CONTINGENCY-BACKUP (2.0TB)",
		"Holographic memory array (128.0TB)",
	} {
		stamp(drive + " ... OK")
	}

	stamp("Initializing secure network subsystem")
	stamp("Network: LUMON-INTERNAL-SEVERED")
	stamp("Establishing protected memory partition")

	for i, n := 0, 8+rng.Intn(5); i < n; i++ {
		stamp("Initializing " + pick(bootDevices))
	}
	for i, n := 0, 15+rng.Intn(10); i < n; i++ {
		stamp("Loading " + pick(bootExtras))
	}

	stamp("KERNEL: Loading LUMON-CORE...")
	for _, s := range bootKernelExtra[:7+rng.Intn(8)] {
		stamp(s)
	}
	for _, s := range bootKernel[:4+rng.Intn(4)] {
		stamp(s)
	}

	for i, n := 0, 6+rng.Intn(6); i < n; i++ {
		stamp(fmt.Sprintf("Loading %s module... %s", pick(bootModules), pick(bootStatuses)))
	}

	add("")
	add(authorizeLine)
	stamp("Verifying user credentials")
	stamp("Biometric scan complete")
	stamp("Partition status: ACTIVE")
	stamp("Authorized: ACCESS LEVEL 3")
	add("")
	stamp("Initializing MDR interface...")
	stamp("Loading refinement matrices...")
	stamp("All systems nominal")
	stamp("SYSTEM READY - INITIATING WORKSPACE")

	return out
}

// sessionID is the hex identifier the terminal claims for this session.
func sessionID(rng *rand.Rand) string {
	return strings.ToUpper(fmt.Sprintf("%08x-%04x-%04x",
		rng.Uint32(), rng.Intn(0xFFFF), rng.Intn(0xFFFF)))
}
