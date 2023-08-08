package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	ACTION_COMMAND_OFFSET    int     = 6
	ACTION_ENTRIES           int     = 8
	ALTERNATE_COUNTERS       int     = 9
	ALTERNATE_ROOM_REGISTERS int     = 6
	AUTO                     int     = 0
	COMMAND_CODE_DIVISOR     int     = 150
	COMMANDS_IN_ACTION       int     = 4
	CONDITION_DIVISOR        int     = 20
	CONDITIONS               int     = 5
	COUNTER_TIME_LIMIT       int     = 8
	DIRECTION_NOUNS          int     = 6
	FALSE_VALUE              int     = 0
	FLAG_LAMP_EMPTY          int     = 16
	FLAG_NIGHT               int     = 15
	LIGHT_SOURCE_ID          int     = 9
	LIGHT_WARNING_THRESHOLD  int     = 25
	MESSAGE_1_END            int     = 51
	MESSAGE_2_START          int     = 102
	MINIMUM_COUNTER_VALUE    int     = -1
	PAR_CONDITION_CODE       int     = 0
	PERCENT_UNITS            int     = 100
	PRNG_PRIME               int     = 65537
	PRNG_PRM                 int     = 75
	REALLY_BIG_NUMBER        int     = 32767
	ROOM_INVENTORY           int     = -1
	ROOM_STORE               int     = 0
	ROUNDING_OFFSET          float64 = 0.5
	STATUS_FLAGS             int     = 32
	VALUES_IN_16_BITS        int     = 65536
	VERB_CARRY               int     = 10
	VERB_DROP                int     = 18
	VERB_GO                  int     = 1
)

var directionNounText = []string{"NORTH", "SOUTH", "EAST", "WEST", "UP", "DOWN"}

var (
	actionData              [][]int
	actionDescription       []string
	adventureNumber         int
	adventureVersion        int
	alternateCounter        []int
	alternateRoom           []int
	carriedObjects          int
	commandInHandle         string
	commandOrDisplayMessage int
	commandOutHandle        string
	commandParameter        int
	commandParameterIndex   int
	contFlag                bool
	counterRegister         int
	currentRoom             int
	extractedInputWords     []string
	flagDebug               bool
	foundWord               []int
	gameBytes               int
	gameFile                string
	globalNoun              string
	keyboardInput           string
	keyboardInput2          string
	listOfVerbsAndNouns     [][]string
	maxObjectsCarried       int
	message                 []string
	numberOfActions         int
	numberOfMessages        int
	numberOfObjects         int
	numberOfRooms           int
	numberOfTreasures       int
	numberOfWords           int
	objectDescription       []string
	objectLocation          []int
	objectOriginalLocation  []int
	roomDescription         []string
	roomExit                [][]int
	startingRoom            int
	statusFlag              []bool
	storedTreasures         int
	timeLimit               int
	treasureRoomId          int
	wordLength              int
)

var (
	prngState = int(time.Now().Unix()) % VALUES_IN_16_BITS
)

var conditionName = []string{
	"Par", "HAS", "IN/W", "AVL", "IN", "-IN/W", "-HAVE", "-IN",
	"BIT", "-BIT", "ANY", "-ANY", "-AVL", "-RM0", "RM0", "CT<=",
	"CT>", "ORIG", "-ORIG", "CT=",
}

var commandName = []string{
	"GETx", "DROPx", "GOTOy", "x->RM0", "NIGHT", "DAY",
	"SETz", "x->RM0", "CLRz", "DEAD", "x->y", "FINI",
	"DspRM", "SCORE", "INV", "SET0", "CLR0", "FILL",
	"CLS", "SAVE", "EXx,x", "CONT", "AGETx", "BYx<-x",
	"DspRM", "CT-1", "DspCT", "CT<-n", "EXRM0", "EXm,CT",
	"CT+n", "CT-n", "SAYw", "SAYwCR", "SAYCR", "EXc,CR",
	"DELAY",
}

type conditionFunc func(int) bool

var conditionFunction []conditionFunc = []conditionFunc{

	// 0 Par
	func(parameter int) bool {
		return true
	},

	// 1 HAS
	func(parameter int) bool {
		return objectLocation[parameter] == ROOM_INVENTORY
	},

	// 2 IN/W
	func(parameter int) bool {
		return objectLocation[parameter] == currentRoom
	},

	// 3 AVL
	func(parameter int) bool {
		return objectLocation[parameter] == ROOM_INVENTORY || objectLocation[parameter] == currentRoom
	},

	// 4 IN
	func(parameter int) bool {
		return currentRoom == parameter
	},

	// 5 -IN/W
	func(parameter int) bool {
		return objectLocation[parameter] != currentRoom
	},

	// 6 -HAVE
	func(parameter int) bool {
		return objectLocation[parameter] != ROOM_INVENTORY
	},

	// 7 -IN
	func(parameter int) bool {
		return currentRoom != parameter
	},

	// 8 BIT
	func(parameter int) bool {
		return statusFlag[parameter]
	},

	// 9 -BIT
	func(parameter int) bool {
		return !statusFlag[parameter]
	},

	// 10 ANY
	func(parameter int) bool {
		for _, location := range objectLocation {
			if location == ROOM_INVENTORY {
				return true
			}
		}
		return false
	},

	// 11 -ANY
	func(parameter int) bool {
		for _, location := range objectLocation {
			if location == ROOM_INVENTORY {
				return false
			}
		}
		return true
	},

	// 12 -AVL
	func(parameter int) bool {
		return !(objectLocation[parameter] == ROOM_INVENTORY || objectLocation[parameter] == currentRoom)
	},

	// 13 -RM0
	func(parameter int) bool {
		return objectLocation[parameter] != ROOM_STORE
	},

	// 14 RM0
	func(parameter int) bool {
		return objectLocation[parameter] == ROOM_STORE
	},

	// 15 CT<=
	func(parameter int) bool {
		return counterRegister <= parameter
	},

	// 16 CT>
	func(parameter int) bool {
		return counterRegister > parameter
	},

	// 17 ORIG
	func(parameter int) bool {
		return objectLocation[parameter] == objectLocation[parameter]
	},

	// 18 -ORIG
	func(parameter int) bool {
		return objectLocation[parameter] != objectLocation[parameter]
	},

	// 19 CT=
	func(parameter int) bool {
		return counterRegister == parameter
	},
}

type commandFunc func(*int, *bool)

var commandFunction []commandFunc = []commandFunc{
	// 0 GETx
	func(actionId *int, continueExecutingCommands *bool) {
		carriedObjects := 0

		for _, location := range objectLocation {
			if location == ROOM_INVENTORY {
				carriedObjects++
			}
		}
		if carriedObjects >= maxObjectsCarried {
			fmt.Println("I've too much too carry. try -take inventory-")
			*continueExecutingCommands = false
		}
		getCommandParameter(*actionId)
		objectLocation[commandParameter] = ROOM_INVENTORY
	},

	// 1 DROPx
	func(actionId *int, continueExecutingCommands *bool) {
		getCommandParameter(*actionId)
		objectLocation[commandParameter] = currentRoom
	},

	// 2 GOTOy
	func(actionId *int, continueExecutingCommands *bool) {
		getCommandParameter(*actionId)
		currentRoom = commandParameter
	},

	// 3 x->RM0
	func(actionId *int, continueExecutingCommands *bool) {
		getCommandParameter(*actionId)
		objectLocation[commandParameter] = 0
	},

	// 4 NIGHT
	func(actionId *int, continueExecutingCommands *bool) {
		statusFlag[FLAG_NIGHT] = true
	},

	// 5 DAY
	func(actionId *int, continueExecutingCommands *bool) {
		statusFlag[FLAG_NIGHT] = false
	},

	// 6 SETz
	func(actionId *int, continueExecutingCommands *bool) {
		getCommandParameter(*actionId)
		statusFlag[commandParameter] = true
	},

	// 7 x->RM0
	func(actionId *int, continueExecutingCommands *bool) {
		getCommandParameter(*actionId)
		objectLocation[commandParameter] = 0
	},

	// 8 CLRz
	func(actionId *int, continueExecutingCommands *bool) {
		getCommandParameter(*actionId)
		statusFlag[commandParameter] = false
	},

	// 9 DEAD
	func(actionId *int, continueExecutingCommands *bool) {
		fmt.Println("I'm dead...")
		currentRoom = numberOfRooms
		statusFlag[FLAG_NIGHT] = false
		showRoomDescription()
	},

	// 10 x->y
	func(actionId *int, continueExecutingCommands *bool) {
		getCommandParameter(*actionId)
		temporary1 := commandParameter
		getCommandParameter(*actionId)
		objectLocation[temporary1] = commandParameter
	},

	// 11 FINI
	func(actionId *int, continueExecutingCommands *bool) {
		os.Exit(0)
	},

	// 12 DspRM
	func(actionId *int, continueExecutingCommands *bool) {
		showRoomDescription()
	},

	// 13 SCORE
	func(actionId *int, continueExecutingCommands *bool) {
		storedTreasures := 0
		for object, location := range objectLocation {
			if location == treasureRoomId {
				if strings.HasPrefix(objectDescription[object], "*") {
					storedTreasures++
				}
			}
		}
		scoreMsg := fmt.Sprintf("I've stored %d treasures. ON A SCALE OF 0 TO %d THAT RATES A %d\n",
			storedTreasures, PERCENT_UNITS,
			int(float64(storedTreasures)/float64(numberOfTreasures)*float64(PERCENT_UNITS)))
		if _, err := fmt.Print(scoreMsg); err != nil {
			log.Fatal(err)
		}
		if storedTreasures == numberOfTreasures {
			fmt.Println("Well done.")
			os.Exit(0)
		}
	},

	// 14 INV
	func(actionId *int, continueExecutingCommands *bool) {
		carryingNothingText := "Nothing"
		objectText := ""
		for object, location := range objectLocation {
			if location != ROOM_INVENTORY {
				continue
			} else {
				objectText = stripNounFromObjectDescription(object)
			}
			if _, err := fmt.Print(objectText, ". "); err != nil {
				log.Fatal(err)
			}
			carryingNothingText = ""
		}
		if _, err := fmt.Print(carryingNothingText, "\n\n"); err != nil {
			log.Fatal(err)
		}
	},

	// 15 SET0
	func(actionId *int, continueExecutingCommands *bool) {
		commandParameter = 0
		statusFlag[commandParameter] = true
	},

	// 16 CLR0
	func(actionId *int, continueExecutingCommands *bool) {
		commandParameter = 0
		statusFlag[commandParameter] = false
	},

	// 17 FILL
	func(actionId *int, continueExecutingCommands *bool) {
		alternateCounter[COUNTER_TIME_LIMIT] = timeLimit
		objectLocation[LIGHT_SOURCE_ID] = ROOM_INVENTORY
		statusFlag[FLAG_LAMP_EMPTY] = false
	},

	// 18 CLS
	func(actionId *int, continueExecutingCommands *bool) {
		cls()
	},

	// 19 SAVE
	func(actionId *int, continueExecutingCommands *bool) {
		saveGame()
	},

	// 20 EXx,x
	func(actionId *int, continueExecutingCommands *bool) {
		getCommandParameter(*actionId)
		temporary1 := commandParameter
		getCommandParameter(*actionId)
		temporary2 := objectLocation[commandParameter]
		objectLocation[commandParameter] = objectLocation[temporary1]
		objectLocation[temporary1] = temporary2
	},

	// 21 CONT
	func(actionId *int, continueExecutingCommands *bool) {
		contFlag = true
	},

	// 22 AGETx
	func(actionId *int, continueExecutingCommands *bool) {
		getCommandParameter(*actionId)
		objectLocation[commandParameter] = ROOM_INVENTORY
	},

	// 23 BYx<-x
	func(actionId *int, continueExecutingCommands *bool) {
		getCommandParameter(*actionId)
		firstObject := commandParameter
		getCommandParameter(*actionId)
		secondObject := commandParameter
		objectLocation[firstObject] = objectLocation[secondObject]
	},

	// 24 DspRM
	func(actionId *int, continueExecutingCommands *bool) {
		showRoomDescription()
	},

	// 25 CT-1
	func(actionId *int, continueExecutingCommands *bool) {
		counterRegister--
	},

	// 26 DspCT
	func(actionId *int, continueExecutingCommands *bool) {
		if _, err := fmt.Print(counterRegister); err != nil {
			log.Fatal(err)
		}
	},

	// 27 CT<-n
	func(actionId *int, continueExecutingCommands *bool) {
		getCommandParameter(*actionId)
		counterRegister = commandParameter
	},

	// 28 EXRM0
	func(actionId *int, continueExecutingCommands *bool) {
		temp := currentRoom
		currentRoom = alternateRoom[0]
		alternateRoom[0] = temp
	},

	// 29 EXm,CT
	func(actionId *int, continueExecutingCommands *bool) {
		getCommandParameter(*actionId)
		temp := counterRegister
		counterRegister = alternateCounter[commandParameter]
		alternateCounter[commandParameter] = temp
	},

	// 30 CT+n
	func(actionId *int, continueExecutingCommands *bool) {
		getCommandParameter(*actionId)
		counterRegister += commandParameter
	},

	// 31 CT-n
	func(actionId *int, continueExecutingCommands *bool) {
		getCommandParameter(*actionId)
		counterRegister -= commandParameter
		if counterRegister < MINIMUM_COUNTER_VALUE {
			counterRegister = MINIMUM_COUNTER_VALUE
		}
	},

	// 32 SAYw
	func(actionId *int, continueExecutingCommands *bool) {
		if _, err := fmt.Print(globalNoun); err != nil {
			log.Fatal(err)
		}
	},

	// 33 SAYwCR
	func(actionId *int, continueExecutingCommands *bool) {
		if _, err := fmt.Print(globalNoun, "\n"); err != nil {
			log.Fatal(err)
		}
	},

	// 34 SAYCR
	func(actionId *int, continueExecutingCommands *bool) {
		if _, err := fmt.Print("\n"); err != nil {
			log.Fatal(err)
		}
	},

	// 35 EXc,CR
	func(actionId *int, continueExecutingCommands *bool) {
		getCommandParameter(*actionId)
		temp := currentRoom
		currentRoom = alternateRoom[commandParameter]
		alternateRoom[commandParameter] = temp
	},

	// 36 DELAY
	func(actionId *int, continueExecutingCommands *bool) {
		time.Sleep(1 * time.Second)
	},
}

func main() {
	// Get commandline options
	commandOrDisplayMessage = 0

	// Load game data file, if specified
	var gameFile string
	argsWithoutProg := os.Args[1:]

	if len(argsWithoutProg) > 0 {
		gameFile = argsWithoutProg[0]
		loadGameDataFile(gameFile)
	} else {
		commandlineHelp()
	}
	// Initialize values
	currentRoom = startingRoom

	// Prepare the rest of the variables
	alternateRoom = make([]int, ALTERNATE_ROOM_REGISTERS)
	alternateCounter = make([]int, ALTERNATE_COUNTERS)
	counterRegister = 0
	statusFlag = make([]bool, STATUS_FLAGS)
	statusFlag[FLAG_NIGHT] = false
	alternateCounter[COUNTER_TIME_LIMIT] = timeLimit

	showIntro()
	showRoomDescription()

	foundWord = []int{0, 0}
	runActions(foundWord[0], 0)

	// Create a new Scanner
	scanner := bufio.NewScanner(os.Stdin)

	//  Main keyboard command input loop
	for {

		fmt.Println("Tell me what to do")

		// Wait for the user to enter a command
		scanner.Scan()

		keyboardInput2 = scanner.Text()
		fmt.Println()

		match, _ := regexp.MatchString(`(?i)^\s*LOAD\s*GAME`, keyboardInput2)

		if match {
			if loadGame() {
				showRoomDescription()
			}
		} else {
			extractWords()

			undefinedWordsFound := (foundWord[0] < 1) ||
				(len(extractedInputWords[1]) > 0) && (foundWord[1] < 1)

			if (foundWord[0] == VERB_CARRY) || (foundWord[0] == VERB_DROP) {
				undefinedWordsFound = false
			}

			if undefinedWordsFound {
				fmt.Println("You use word(s) I don't know")
			} else {
				runActions(foundWord[0], foundWord[1])
				checkAndChangeLightSourceStatus()
				foundWord[0] = 0
				runActions(foundWord[0], foundWord[1])
			}
		}
	}
}

func getPrn() int {
	prngState = (PRNG_PRM * (prngState + 1) % PRNG_PRIME) % VALUES_IN_16_BITS
	return prngState % PERCENT_UNITS
}

var inputReader *bufio.Reader = bufio.NewReader(os.Stdin)

func getCommandInput() string {
	input, err := inputReader.ReadString('\n')
	if err != nil && err != io.EOF {
		panic(err)
	}
	return input
}

func commandlineHelp() {
	fmt.Println(`
Usage: perlscott.pl [OPTION]... game_data_file
Scott Adams adventure game interpreter

-i, --input    Command input file
-o, --output   Command output file
-d, --debug    Show game debugging info
-h, --help     Display this help and exit`)
	os.Exit(0)
}

func commandlineOptions() (*os.File, *os.File, bool) {
	inputFile := flag.String("i", "", "Command input file")
	outputFile := flag.String("o", "", "Command output file")
	debug := flag.Bool("d", false, "Show game debugging info")
	help := flag.Bool("h", false, "Display this help and exit")
	flag.Parse()

	if *help {
		commandlineHelp()
	}

	var inHandle *os.File = os.Stdin
	if *inputFile != "" {
		var err error
		inHandle, err = os.Open(*inputFile)
		if err != nil {
			panic(fmt.Sprintf("file \"%s\" not found", *inputFile))
		}
		inputReader = bufio.NewReader(inHandle)
	}

	var outHandle *os.File
	if *outputFile != "" {
		var err error
		outHandle, err = os.OpenFile(*outputFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			panic(err)
		}
	}

	return inHandle, outHandle, *debug
}

func stripNounFromObjectDescription(objectNumber int) string {
	strippedText := objectDescription[objectNumber]
	re := regexp.MustCompile(`/.*/`)
	strippedText = re.ReplaceAllString(strippedText, "")
	return strippedText
}

func checkAndChangeLightSourceStatus() int {
	if objectLocation[LIGHT_SOURCE_ID] == ROOM_INVENTORY {
		alternateCounter[COUNTER_TIME_LIMIT]--
		if alternateCounter[COUNTER_TIME_LIMIT] < 0 {
			fmt.Println("Light has run out")
			objectLocation[LIGHT_SOURCE_ID] = 0
		} else if alternateCounter[COUNTER_TIME_LIMIT] < LIGHT_WARNING_THRESHOLD {
			fmt.Printf("Light runs out in %d turns!\n", alternateCounter[COUNTER_TIME_LIMIT])
		}
	}
	return 1
}

func showIntro() int {
	cls() // Clear screen commented out for debugging reasons
	introMessage := `
                 *** Welcome ***

 Unless told differently you must find *TREASURES* 
and-return-them-to-their-proper--place!

I'm your puppet. Give me english commands that
consist of a noun and verb. Some examples...

To find out what you're carrying you might say: TAKE INVENTORY 
to go into a hole you might say: GO HOLE 
to save current game: SAVE GAME

You will at times need special items to do things: But I'm 
sure you'll be a good adventurer and figure these things out.

     Happy adventuring... Hit enter to start`
	fmt.Println(introMessage)

	keyboardInput = getCommandInput()
	cls()
	return 1
}

func showRoomDescription() int {
	if statusFlag[FLAG_NIGHT] != false {
		if objectLocation[LIGHT_SOURCE_ID] != ROOM_INVENTORY && objectLocation[LIGHT_SOURCE_ID] != currentRoom {
			fmt.Println("I can't see: Its too dark.")
			return 1
		}
	}

	if strings.HasPrefix(roomDescription[currentRoom], "*") {
		fmt.Println(roomDescription[currentRoom][1:])
	} else {
		fmt.Printf("I'm in a %s", roomDescription[currentRoom])
	}

	objectsFound := false
	for i, location := range objectLocation {
		if location == currentRoom {
			if !objectsFound {
				fmt.Print(". Visible items here: \n")
				objectsFound = true
			}
			fmt.Printf("%s. ", stripNounFromObjectDescription(i))
		}
	}
	fmt.Println()

	exitFound := false
	for i, exit := range roomExit[currentRoom] {
		if exit != 0 {
			if !exitFound {
				fmt.Print("Obvious exits: ")
				exitFound = true
			}
			fmt.Printf("%s ", directionNounText[i])
		}
	}
	fmt.Print("\n\n")
	return 1
}

func handleGoVerb() int {
	roomDark := statusFlag[FLAG_NIGHT]
	if roomDark {
		roomDark = objectLocation[LIGHT_SOURCE_ID] != currentRoom && objectLocation[LIGHT_SOURCE_ID] != 1
		if roomDark {
			fmt.Println("Dangerous to move in the dark!")
		}
	}

	if foundWord[1] < 1 {
		fmt.Println("Give me a direction too.")
		return 1
	}

	directionDestination := roomExit[currentRoom][foundWord[1]-1]
	if directionDestination < 1 {
		if roomDark {
			fmt.Println("I fell down and broke my neck.")
			directionDestination = numberOfRooms
			statusFlag[FLAG_NIGHT] = false
		} else {
			fmt.Println("I can't go in that direction")
			return 1
		}
	}

	currentRoom = directionDestination
	showRoomDescription()
	return 1
}

// Get command parameter from the condition section
func getCommandParameter(currentAction int) (int, error) {
	var conditionCode int = 1
	for conditionCode != PAR_CONDITION_CODE {
		conditionLine := actionData[currentAction][commandParameterIndex]
		commandParameter = int(conditionLine / CONDITION_DIVISOR)
		conditionCode = conditionLine - commandParameter*CONDITION_DIVISOR
		commandParameterIndex++
	}

	return 1, nil
}

func decodeCommandFromData(commandNumber, actionId int) int {
	mergedCommandIndex := int(commandNumber/2 + ACTION_COMMAND_OFFSET)
	var commandCode int
	// Even or odd command number?
	if commandNumber%2 != 0 {
		commandCode = actionData[actionId][mergedCommandIndex] - int(actionData[actionId][mergedCommandIndex]/COMMAND_CODE_DIVISOR)*COMMAND_CODE_DIVISOR
	} else {
		commandCode = int(actionData[actionId][mergedCommandIndex] / COMMAND_CODE_DIVISOR)
	}
	return commandCode
}

func matchUnixNewline(input string) bool {
	// Look for a \r\n or \n\r sequence
	crlfPattern := regexp.MustCompile(`\r\n|\n\r`)
	if crlfPattern.MatchString(input) {
		// If we found it, return false
		return false
	}

	// Look for a standalone \n
	lfPattern := regexp.MustCompile(`\n`)
	return lfPattern.MatchString(input)
}

func normalizeNewlines(input string, desiredNewline string) string {
	input = strings.Replace(input, "\r\n", "\n", -1) // Convert DOS to Unix
	input = strings.Replace(input, "\r", "\n", -1)   // Convert Apple to Unix

	return strings.Replace(input, "\n", desiredNewline, -1) // Convert to desired newline
}

func loadGameDataFile(gameFile string) error {
	// Read game file
	fileContentBytes, err := ioutil.ReadFile(gameFile)
	if err != nil {
		return err
	}

	// Convert bytes to string
	fileContent := string(fileContentBytes)

	// Replace newline with current system newline
	fileContent = normalizeNewline(fileContent)

	// Regular expressions for data extraction
	roomPattern := regexp.MustCompile(`\s+(-?\d+)\s+(-?\d+)\s+(-?\d+)\s+(-?\d+)\s+(-?\d+)\s+(-?\d+)\s*"([^"]*)"([\s\S]*)`)
	objectPattern := regexp.MustCompile(`\s*"([^"]*)"\s*(-?\d+)([\s\S]*)`)
	wordPattern := regexp.MustCompile(`\s*"([*]?[^"]*?)"([\s\S]*)`)
	textPattern := regexp.MustCompile(`\s*"([^"]*)"([\s\S]*)`)

	next := fileContent
	gameBytes, next = extractInt(next)
	numberOfObjects, next = extractInt(next)
	numberOfActions, next = extractInt(next)
	numberOfWords, next = extractInt(next)
	numberOfRooms, next = extractInt(next)
	maxObjectsCarried, next = extractInt(next)
	if maxObjectsCarried < 0 {
		maxObjectsCarried = REALLY_BIG_NUMBER
	}
	startingRoom, next = extractInt(next)
	numberOfTreasures, next = extractInt(next)
	wordLength, next = extractInt(next)
	timeLimit, next = extractInt(next)
	numberOfMessages, next = extractInt(next)
	treasureRoomId, next = extractInt(next)

	// Extract actions
	actionId := 0
	for actionId <= numberOfActions {
		actionIdEntry := 0
		// Iterate over the 8 number values that make up an encoded action
		var entryInAction []int
		for actionIdEntry < ACTION_ENTRIES {
			var actionEntryValue int
			actionEntryValue, next = extractInt(next)
			entryInAction = append(entryInAction, actionEntryValue)
			actionIdEntry++
		}
		actionData = append(actionData, entryInAction)
		actionId++
	}

	// Extract words
	listOfVerbsAndNouns = make([][]string, numberOfWords+1)
	word := 0
	for word < ((numberOfWords + 1) * 2) {
		var input string
		input, next = extractString(next, wordPattern)
		listOfVerbsAndNouns[word/2] = append(listOfVerbsAndNouns[word/2], input)
		word++
	}

	// Extract rooms
	room := 0
	for room <= numberOfRooms {
		matches := roomPattern.FindStringSubmatch(next)
		if len(matches) > 0 {
			var exit []int
			for i := 0; i < 6; i++ {
				exitNumber, _ := strconv.Atoi(matches[i+1])
				exit = append(exit, exitNumber)
			}
			roomExit = append(roomExit, exit)
			roomDescription = append(roomDescription, matches[7])
			next = matches[8]
		}
		room++
	}

	// Extract messages
	currentMessage := 0
	for currentMessage <= numberOfMessages {
		var messageText string
		messageText, next = extractString(next, textPattern)
		message = append(message, messageText)
		currentMessage++
	}

	// Extract objects
	object := 0
	for object <= numberOfObjects {
		matches := objectPattern.FindStringSubmatch(next)
		if len(matches) > 0 {
			objectDescription = append(objectDescription, matches[1])
			location, _ := strconv.Atoi(matches[2])
			objectLocation = append(objectLocation, location)
			objectOriginalLocation = append(objectOriginalLocation, location)
			next = matches[3]

		}
		object++
	}

	// Extract action descriptions
	actionCounter := 0
	for actionCounter <= numberOfActions {
		var descriptionText string
		descriptionText, next = extractString(next, textPattern)
		actionDescription = append(actionDescription, descriptionText)
		actionCounter++
	}

	// Extract adventure version and number
	adventureVersion, next = extractInt(next)
	adventureNumber, next = extractInt(next)

	// Replace Ascii 96 with Ascii 34 in output text strings
	replaceInStringSlice(objectDescription, "`", `"`)
	replaceInStringSlice(message, "`", `"`)
	replaceInStringSlice(roomDescription, "`", `"`)

	return nil
}

func replaceInStringSlice(slice []string, old, new string) {
	for i := range slice {
		slice[i] = strings.Replace(slice[i], old, new, -1)
	}
}

func extractString(s string, re *regexp.Regexp) (string, string) {
	matches := re.FindStringSubmatch(s)
	if len(matches) > 0 {
		return matches[1], matches[2]
	}
	return "", s
}

func extractInt(s string) (int, string) {
	re := regexp.MustCompile(`\s*(-?\d+)([\s\S]*)`)
	matches := re.FindStringSubmatch(s)
	if len(matches) > 0 {
		value, _ := strconv.Atoi(matches[1])
		return value, matches[2]
	}
	return 0, s
}

func normalizeNewline(s string) string {
	return strings.Replace(s, "\r\n", "\n", -1)
}

func cls() bool {
	fmt.Print("\033[H\033[2J")
	return true
}

func extractWords() int {
	//Reset extractedInputWords slice
	extractedInputWords = []string{}

	// Trim leading white spaces
	keyboardInput2 = strings.TrimLeft(keyboardInput2, " ")

	//Split keyboardInput2 into words
	extractedInputWords = strings.Split(keyboardInput2, " ")

	if len(extractedInputWords) == 0 {
		extractedInputWords = append(extractedInputWords, "")
	}

	resolveGoShortcut()

	// If the length of extractedInputWords is less than 2, add an empty string
	if len(extractedInputWords) < 2 {
		extractedInputWords = append(extractedInputWords, "")
	}
	globalNoun = extractedInputWords[1]

	//Reset foundWord slice
	foundWord = []int{0, 0}

	for verbOrNoun := 0; verbOrNoun <= 1; verbOrNoun++ {
		nonSynonym := 0
		for wordId, word := range listOfVerbsAndNouns {
			if strings.Index(word[verbOrNoun], "*") != 0 {
				nonSynonym = wordId
			}
			tempWord := strings.TrimLeft(word[verbOrNoun], "*")
			tempWord = extractFirstCharacters(tempWord, wordLength)
			if tempWord == strings.ToUpper(extractFirstCharacters(extractedInputWords[verbOrNoun], wordLength)) {
				foundWord[verbOrNoun] = nonSynonym
				break
			}
		}
	}
	return 1
}

func extractFirstCharacters(input string, limit int) string {
	if len(input) >= limit {
		return input[:limit]
	}
	return input
}

func boolSliceToIntSlice(boolSlice []bool) []int {
	intSlice := make([]int, len(boolSlice))
	for i, val := range boolSlice {
		if val {
			intSlice[i] = 1
		} else {
			intSlice[i] = 0
		}
	}
	return intSlice
}

func saveGame() bool {
	fmt.Println("Name of save file:")
	reader := bufio.NewReader(os.Stdin)
	saveFileName, _ := reader.ReadString('\n')
	saveFileName = strings.TrimSuffix(saveFileName, "\n")

	saveData := []int{adventureVersion, adventureNumber, currentRoom}
	saveData = append(saveData, alternateRoom...)
	saveData = append(saveData, counterRegister)
	saveData = append(saveData, alternateCounter...)
	saveData = append(saveData, objectLocation...)
	statusFlagInt := boolSliceToIntSlice(statusFlag)
	saveData = append(saveData, statusFlagInt...)

	saveFile, _ := os.Create(saveFileName)
	for _, data := range saveData {
		fmt.Fprintln(saveFile, data)
	}
	saveFile.Close()

	return true
}

func intToBool(num int) bool {
	if num == FALSE_VALUE {
		return false
	} else {
		return true
	}
}

func loadGame() bool {
	fmt.Println("Name of save file:")
	reader := bufio.NewReader(os.Stdin)
	saveFileName, _ := reader.ReadString('\n')
	saveFileName = strings.TrimSuffix(saveFileName, "\n")

	saveFile, err := os.Open(saveFileName)
	if err != nil {
		fmt.Printf("Couldn't load \"%s\". Doesn't exist!\n", saveFileName)
		return false
	}

	var saveData []int
	scanner := bufio.NewScanner(saveFile)
	for scanner.Scan() {
		var num int
		fmt.Sscan(scanner.Text(), &num)
		saveData = append(saveData, num)
	}

	saveAdventureVersion := saveData[0]
	if saveAdventureVersion != adventureVersion {
		fmt.Println("Invalid savegame version")
		return false
	}

	saveAdventureNumber := saveData[1]
	if saveAdventureNumber != adventureNumber {
		fmt.Println("Invalid savegame adventure number")
		return false
	}

	currentRoom = saveData[2]
	for i := range alternateRoom {
		alternateRoom[i] = saveData[i+3]
	}
	counterRegister = saveData[3+len(alternateRoom)]
	for i := range alternateCounter {
		alternateCounter[i] = saveData[i+4+len(alternateRoom)]
	}
	for i := range objectLocation {
		objectLocation[i] = saveData[i+4+len(alternateRoom)+len(alternateCounter)]
	}
	for i := range statusFlag {
		statusFlag[i] = intToBool(saveData[i+4+len(alternateRoom)+len(alternateCounter)+len(objectLocation)])
	}

	return true
}

func runActions(inputVerb int, inputNoun int) bool {
	if inputVerb == VERB_GO && inputNoun <= DIRECTION_NOUNS {
		handleGoVerb()
		return true
	}

	foundWord := false

	contFlag = false
	wordActionDone := false
	for currentAction, _ := range actionDescription {
		actionVerb := getActionVerb(currentAction)
		actionNoun := getActionNoun(currentAction)

		// CONT action
		if contFlag && actionVerb == 0 && actionNoun == 0 {
			if evaluateConditions(currentAction) {
				executeCommands(currentAction)
			}
		} else {
			contFlag = false
		}

		// AUT action
		if inputVerb == 0 {
			if actionVerb == 0 && actionNoun > 0 {
				contFlag = false
				if getPrn() < actionNoun {
					if evaluateConditions(currentAction) {
						executeCommands(currentAction)
					}
				}
			}
		}

		// Word action
		if inputVerb > 0 {
			if actionVerb == inputVerb {
				if wordActionDone == false {
					contFlag = false
					if actionNoun == 0 {
						foundWord = true
						if evaluateConditions(currentAction) {
							executeCommands(currentAction)
							wordActionDone = true
							if contFlag == false {
								return true
							}
						}
					} else if actionNoun == inputNoun {
						foundWord = true
						if evaluateConditions(currentAction) {
							executeCommands(currentAction)
							wordActionDone = true
							if contFlag == false {
								return true
							}
						}
					}
				}
			}
		}
	}

	if inputVerb == 0 {
		return true
	}

	if wordActionDone == false {
		if handleCarryAndDropVerb(inputVerb, inputNoun) {
			return true
		}
	}

	if wordActionDone {
		return true
	}

	if foundWord {
		fmt.Println("I can't do that yet")
	} else {
		fmt.Println("I don't understand your command")
	}

	return true
}

func nounIsInObject() bool {
	truncatedNoun := globalNoun[:wordLength]
	for _, description := range objectDescription {
		if strings.Contains(description, "/") {
			objectNoun := strings.ToLower(strings.Split(description, "/")[1])
			if objectNoun == truncatedNoun {
				return true
			}
		}
	}
	return false
}

func handleCarryAndDropVerb(inputVerb, inputNoun int) bool {
	// Exit function if the verb isn't carry or drop
	if inputVerb != VERB_CARRY && inputVerb != VERB_DROP {
		return false
	}

	// If noun is undefined, return with an error text
	if inputNoun == 0 && !nounIsInObject() {
		fmt.Println("What?")
		return true
	}

	// If verb is CARRY, check that we're not exceeding weight limit
	if inputVerb == VERB_CARRY {
		carriedObjects := 0

		for _, location := range objectLocation {
			if location == ROOM_INVENTORY {
				carriedObjects++
			}
		}

		if carriedObjects >= maxObjectsCarried {
			if maxObjectsCarried >= 0 {
				fmt.Println("I've too much too carry. try -take inventory-")
				return true
			}
		} else {
			if getOrDropNoun(inputNoun, currentRoom, ROOM_INVENTORY) {
				return true
			} else {
				fmt.Println("I don't see it here")
				return true
			}
		}
	} else {
		if getOrDropNoun(inputNoun, ROOM_INVENTORY, currentRoom) {
			return true
		} else {
			fmt.Println("I'm not carrying it")
			return true
		}
	}

	return false
}

func getOrDropNoun(inputNoun, roomSource, roomDestination int) bool {
	var objectsInRoom []int
	objectCounter := 0

	// Identify all objects in current room
	for _, location := range objectLocation {
		if location == roomSource {
			objectsInRoom = append(objectsInRoom, objectCounter)
		}
		objectCounter++
	}

	// Check if any of the objects in the room has a matching noun
	for _, roomObject := range objectsInRoom {

		// Only proceed if the object has a noun defined
		if strings.Contains(objectDescription[roomObject], "/") {

			// Pick up the first object we find that matches and return
			noun := strings.Split(objectDescription[roomObject], "/")[1]
			if listOfVerbsAndNouns[inputNoun][1] == noun || noun == strings.ToUpper(globalNoun[:wordLength]) {
				objectLocation[roomObject] = roomDestination
				fmt.Println("OK")
				return true
			}
		}
	}
	return false
}

func getActionVerb(actionId int) int {
	return actionData[actionId][0] / COMMAND_CODE_DIVISOR
}

func getActionNoun(actionId int) int {
	return actionData[actionId][0] % COMMAND_CODE_DIVISOR
}

func executeCommands(actionId int) int {
	commandParameterIndex = 1
	command := 0
	continueExecutingCommands := true

	for command < COMMANDS_IN_ACTION && continueExecutingCommands {
		commandOrDisplayMessage := decodeCommandFromData(command, actionId)
		command++

		// Code above 102? it's printable text!
		if commandOrDisplayMessage >= MESSAGE_2_START {
			fmt.Println(message[commandOrDisplayMessage-MESSAGE_1_END+1])
		} else if commandOrDisplayMessage == 0 {
			// Do nothing
		} else if commandOrDisplayMessage <= MESSAGE_1_END {
			// Code below 52? it's printable text!
			fmt.Println(message[commandOrDisplayMessage])
		} else {
			// Code above 52 and below 102? We got some command code to run!
			commandCode := commandOrDisplayMessage - MESSAGE_1_END - 1
			// Launch execution of action commands
			commandFunction[commandCode](&actionId, &continueExecutingCommands)
		}
	}

	return 1
}

func evaluateConditions(actionId int) bool {
	evaluationStatus := true
	condition := 1
	for condition <= CONDITIONS {
		conditionCode := getConditionCode(actionId, condition)
		conditionParameter := getConditionParameter(actionId, condition)
		condition_success := conditionFunction[conditionCode](conditionParameter)
		if !condition_success {
			// Stop evaluating conditions if false. One fails all.
			evaluationStatus = false
			break
		}
		condition++
	}
	return evaluationStatus
}

func getConditionCode(actionId int, condition int) int {
	conditionRaw := actionData[actionId][condition]
	conditionCode := conditionRaw % CONDITION_DIVISOR
	return conditionCode
}

func getConditionParameter(actionId int, condition int) int {
	conditionRaw := actionData[actionId][condition]
	conditionParameter := conditionRaw / CONDITION_DIVISOR
	return conditionParameter
}

func resolveGoShortcut() int {
	enteredInputVerb := strings.ToLower(extractedInputWords[0])
	viablePhrases := getViableWordActions()

	// Don't attempt to resolve go shortcuts if input is empty
	if len(enteredInputVerb) < 1 {
		return 1
	}

	// Don't make shortcut if input verb matches legitimate word action
	for viableVerb := range viablePhrases {
		possibleVerbText := strings.ToLower(listOfVerbsAndNouns[viableVerb][0])
		shortenedVerb := enteredInputVerb
		if len(possibleVerbText) < len(enteredInputVerb) {
			shortenedVerb = enteredInputVerb[0:len(possibleVerbText)]
		}
		if shortenedVerb == possibleVerbText {
			return 1
		}
	}

	for direction := 1; direction <= DIRECTION_NOUNS; direction++ {
		directionNounText := strings.ToLower(listOfVerbsAndNouns[direction][1])
		shortenedDirection := directionNounText
		if len(enteredInputVerb) < len(directionNounText) {
			shortenedDirection = directionNounText[0:len(enteredInputVerb)]
		}

		if enteredInputVerb == shortenedDirection {
			extractedInputWords[0] = strings.ToLower(listOfVerbsAndNouns[VERB_GO][0])
			extractedInputWords = append(extractedInputWords, directionNounText)
			return 1
		}
	}
	return 1
}

func getViableWordActions() map[int]map[int]string {
	viablePhrases := make(map[int]map[int]string)
	currentAction := 0
	for range actionData {
		actionVerb := getActionVerb(currentAction)
		actionNoun := getActionNoun(currentAction)
		if actionVerb > 0 {
			if evaluateConditions(currentAction) {
				if _, ok := viablePhrases[actionVerb]; !ok {
					viablePhrases[actionVerb] = make(map[int]string)
				}
				viablePhrases[actionVerb][actionNoun] = ""
			}
		}
		currentAction++
	}
	return viablePhrases
}
