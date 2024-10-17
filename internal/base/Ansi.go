package base

import (
	"math/rand"
	"strconv"
)

type AnsiCode string

var enableAnsiColor bool = true

func SetEnableAnsiColor(enabled bool) {
	enableAnsiColor = enabled
}

func (x AnsiCode) Always() string {
	return (string)(x)
}
func (x AnsiCode) String() string {
	if enableAnsiColor {
		return (string)(x)
	}
	return ""
}

// https://gist.github.com/fnky/458719343aabd01cfb17a3a4f7296797

const (
	ANSI_RESET     AnsiCode = "\033[0m"
	ANSI_BOLD      AnsiCode = "\033[1m"
	ANSI_FAINT     AnsiCode = "\033[2m"
	ANSI_ITALIC    AnsiCode = "\033[3m"
	ANSI_UNDERLINE AnsiCode = "\033[4m"
	ANSI_BLINK0    AnsiCode = "\033[5m"
	ANSI_BLINK1    AnsiCode = "\033[6m"
	ANSI_REVERSED  AnsiCode = "\033[7m"
	ANSI_FRAME     AnsiCode = "\033[51m"
	ANSI_ENCIRCLE  AnsiCode = "\033[52m"
	ANSI_OVERLINE  AnsiCode = "\033[53m"

	ANSI_FG0_BLACK   AnsiCode = "\033[30m"
	ANSI_FG0_RED     AnsiCode = "\033[31m"
	ANSI_FG0_GREEN   AnsiCode = "\033[32m"
	ANSI_FG0_YELLOW  AnsiCode = "\033[33m"
	ANSI_FG0_BLUE    AnsiCode = "\033[34m"
	ANSI_FG0_MAGENTA AnsiCode = "\033[35m"
	ANSI_FG0_CYAN    AnsiCode = "\033[36m"
	ANSI_FG0_WHITE   AnsiCode = "\033[37m"
	ANSI_FG1_BLACK   AnsiCode = "\033[30;1m"
	ANSI_FG1_RED     AnsiCode = "\033[31;1m"
	ANSI_FG1_GREEN   AnsiCode = "\033[32;1m"
	ANSI_FG1_YELLOW  AnsiCode = "\033[33;1m"
	ANSI_FG1_BLUE    AnsiCode = "\033[34;1m"
	ANSI_FG1_MAGENTA AnsiCode = "\033[35;1m"
	ANSI_FG1_CYAN    AnsiCode = "\033[36;1m"
	ANSI_FG1_WHITE   AnsiCode = "\033[37;1m"
	ANSI_BG0_BLACK   AnsiCode = "\033[40m"
	ANSI_BG0_RED     AnsiCode = "\033[41m"
	ANSI_BG0_GREEN   AnsiCode = "\033[42m"
	ANSI_BG0_YELLOW  AnsiCode = "\033[43m"
	ANSI_BG0_BLUE    AnsiCode = "\033[44m"
	ANSI_BG0_MAGENTA AnsiCode = "\033[45m"
	ANSI_BG0_CYAN    AnsiCode = "\033[46m"
	ANSI_BG0_WHITE   AnsiCode = "\033[47m"
	ANSI_BG1_BLACK   AnsiCode = "\033[40;1m"
	ANSI_BG1_RED     AnsiCode = "\033[41;1m"
	ANSI_BG1_GREEN   AnsiCode = "\033[42;1m"
	ANSI_BG1_YELLOW  AnsiCode = "\033[43;1m"
	ANSI_BG1_BLUE    AnsiCode = "\033[44;1m"
	ANSI_BG1_MAGENTA AnsiCode = "\033[45;1m"
	ANSI_BG1_CYAN    AnsiCode = "\033[46;1m"
	ANSI_BG1_WHITE   AnsiCode = "\033[47;1m"

	ANSI_ERASE_END_LINE           AnsiCode = "\033[K"
	ANSI_ERASE_START_LINE         AnsiCode = "\033[1K"
	ANSI_ERASE_ALL_LINE           AnsiCode = "\033[2K"
	ANSI_ERASE_SCREEN_FROM_CURSOR AnsiCode = "\033[0J"
	ANSI_ERASE_SCREEN_TO_CURSOR   AnsiCode = "\033[1J"

	ANSI_CURSOR_UP        AnsiCode = "\033[A"
	ANSI_CURSOR_PREV_LINE AnsiCode = "\033[F"

	ANSI_BG_TRUECOLOR_FMT AnsiCode = "\033[48;2;%v;%v;%vm"
	ANSI_FG_TRUECOLOR_FMT AnsiCode = "\033[38;2;%v;%v;%vm"
)

var (
	ANSI_COLORS = [7]string{
		"black", "red", "green", "yellow", "blue", "magenta", "cyan",
	}

	ANSI_CODES = map[string]AnsiCode{
		"fg0_black":   ANSI_FG0_BLACK,
		"fg0_red":     ANSI_FG0_RED,
		"fg0_green":   ANSI_FG0_GREEN,
		"fg0_yellow":  ANSI_FG0_YELLOW,
		"fg0_blue":    ANSI_FG0_BLUE,
		"fg0_magenta": ANSI_FG0_MAGENTA,
		"fg0_cyan":    ANSI_FG0_CYAN,
		"fg0_white":   ANSI_FG0_WHITE,
		"fg1_black":   ANSI_FG1_BLACK,
		"fg1_red":     ANSI_FG1_RED,
		"fg1_green":   ANSI_FG1_GREEN,
		"fg1_yellow":  ANSI_FG1_YELLOW,
		"fg1_blue":    ANSI_FG1_BLUE,
		"fg1_magenta": ANSI_FG1_MAGENTA,
		"fg1_cyan":    ANSI_FG1_CYAN,
		"fg1_white":   ANSI_FG1_WHITE,
		"bg0_black":   ANSI_BG0_BLACK,
		"bg0_red":     ANSI_BG0_RED,
		"bg0_green":   ANSI_BG0_GREEN,
		"bg0_yellow":  ANSI_BG0_YELLOW,
		"bg0_blue":    ANSI_BG0_BLUE,
		"bg0_magenta": ANSI_BG0_MAGENTA,
		"bg0_cyan":    ANSI_BG0_CYAN,
		"bg0_white":   ANSI_BG0_WHITE,
		"bg1_black":   ANSI_BG1_BLACK,
		"bg1_red":     ANSI_BG1_RED,
		"bg1_green":   ANSI_BG1_GREEN,
		"bg1_yellow":  ANSI_BG1_YELLOW,
		"bg1_blue":    ANSI_BG1_BLUE,
		"bg1_magenta": ANSI_BG1_MAGENTA,
		"bg1_cyan":    ANSI_BG1_CYAN,
		"bg1_white":   ANSI_BG1_WHITE,
	}
)

func ansi_escaped_len(in string) int {
	if !enableAnsiColor {
		return len(in)
	}

	n := 0
	ignore := false
	for _, ch := range in {
		if ignore {
			ignore = (ch != 'm')
		} else {
			ignore = (ch == '\033')
			if !ignore && strconv.IsGraphic(ch) {
				n++
			}
		}
	}
	return n
}

//lint:ignore U1000 ignore unused function
func make_ansi_color(prefix string, color string) AnsiCode {
	if enableAnsiColor {
		return ANSI_CODES[prefix+"_"+color]
	} else {
		return AnsiCode("")
	}
}

var UnicodeEmojisShuffled = func() (result []rune) {
	result = CopySlice(UnicodeEmojis...)
	rand.Shuffle(len(result), func(i, j int) {
		result[i], result[j] = result[j], result[i]
	})
	return
}()

var UnicodeEmojis = []rune{
	0x1F300, // Cyclone
	0x1F302, // Closed Umbrella
	0x1F308, // Rainbow
	0x1F30A, // Water Wave
	0x1F30B, // Volcano
	0x1F30C, // Milky Way
	0x1F31F, // Glowing Star
	0x1F320, // Shooting Star
	0x1F330, // Chestnut
	0x1F331, // Seeding
	0x1F332, // Evergreen Tree
	0x1F333, // Deciduous Tree
	0x1F334, // Palm Tree
	0x1F335, // Cactus
	0x1F337, // Tulip
	0x1F338, // Cherry Blossom
	0x1F339, // Rose
	0x1F33A, // Hibiscus
	0x1F33B, // Sunflower
	0x1F33C, // Blossom
	0x1F33D, // Ear of Maize
	0x1F33E, // Ear of Rice
	0x1F33F, // Herb
	0x1F340, // Four Leaf Clover
	0x1F341, // Maple Leaf
	0x1F342, // Fallen Leaf
	0x1F343, // Leaf Fluttering in Wind
	0x1F344, // Mushroom
	0x1F345, // Tomato
	0x1F346, // Aubergin
	0x1F347, // Grapes
	0x1F348, // Melon
	0x1F349, // Watermelon
	0x1F34A, // Tangerine
	0x1F34B, // Lemon
	0x1F34C, // Banana
	0x1F34D, // Pineapple
	0x1F34E, // Red Apple
	0x1F34F, // Green Apple
	0x1F350, // Pear
	0x1F351, // Peach
	0x1F352, // Cherries
	0x1F353, // Strawberry
	0x1F354, // Hamburger
	0x1F355, // Slice of Pizza
	0x1F356, // Meat on Bone
	0x1F357, // Poultry Leg
	0x1F358, // Rice Cracker
	0x1F359, // Rice Ball
	0x1F35A, // Cooked Rice
	0x1F35B, // Curry and Rice
	0x1F35C, // Steaming Bowl
	0x1F35D, // Spaghetti
	0x1F35E, // Roasted Sweet Potato
	0x1F35F, // Dango
	0x1F360, // Roasted Sweet Potato
	0x1F361, // Odeng
	0x1F362, // Sushi
	0x1F363, // Fish Cake with Swirl Design
	0x1F364, // Soft Ice Cream
	0x1F365, // Ice Cream
	0x1F366, // Ice Cream
	0x1F367, // Shaved Ice
	0x1F368, // Ice Cream
	0x1F369, // Doughnut
	0x1F36A, // Cookie
	0x1F36B, // Chocolate Bar
	0x1F36C, // Candy
	0x1F36D, // Lollipop
	0x1F36E, // Custard
	0x1F36F, // Honey Pot
	0x1F370, // Shortcake
	0x1F371, // Bento Box
	0x1F372, // Pot of Food
	0x1F373, // Cooking
	0x1F374, // Fork and Knife
	0x1F375, // Teacup Without Handle
	0x1F376, // Sake Bottle and Cup
	0x1F377, // Wine Glass
	0x1F378, // Cocktail Glass
	0x1F379, // Tropical Drink
	0x1F37A, // Beer Mug
	0x1F37B, // Clinking Beer Mugs
	0x1F37C, // Baby Bottle
	0x1F37E, // Bottle with Popping Cork
	0x1F37F, // Popcorn
	0x1F380, // Ribbon
	0x1F381, // Wrapped Present
	0x1F382, // Birthday Cake
	0x1F383, // Jack-O-Lantern
	0x1F384, // Christmas Tree
	0x1F385, // Father Christmas
	0x1F386, // Fireworks
	0x1F388, // Balloon
	0x1F389, // Party Popper
	0x1F38A, // Confetti Ball
	0x1F38B, // Tanabata Tree
	0x1F38C, // Crossed Flags
	0x1F38D, // Pine Decoration
	0x1F38E, // Japanese Dolls
	0x1F38F, // Carp Streamer
	0x1F390, // Wind Chime
	0x1F391, // Moon Viewing Ceremony
	0x1F392, // School Satchel
	0x1F393, // Graduation Cap
	0x1F3A0, // Carousel Horse
}
