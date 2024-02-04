package base

import (
	"fmt"
	"math/big"
	"strings"
	"unicode"
)

var LogAutoComplete = NewLogCategory("AutoComplete")

type StringSimilarity interface {
	Input() string
	Score(candidate string) float32
}

/***************************************
 * Jaro-Winkler similarity
 ***************************************/

type JaroWinklerSimilarity struct {
	input           []rune
	MaxPrefixLength int
}

func NewJaroWinklerSimilarity(input string, maxPrefixLength int) JaroWinklerSimilarity {
	return JaroWinklerSimilarity{
		input:           []rune(strings.ToUpper(input)),
		MaxPrefixLength: maxPrefixLength, // You can adjust this value based on your preferences
	}
}

func (x JaroWinklerSimilarity) Input() string {
	return string(x.input)
}
func (x JaroWinklerSimilarity) Score(candidate string) float32 {
	return jaroWinklerSimilarity([]rune(strings.ToUpper(candidate)), x.input, x.MaxPrefixLength)
}

func jaroWinklerSimilarity(s1, s2 []rune, maxPrefixLength int) float32 {
	// Jaro similarity
	jaroSimilarity := jaroSimilarity(s1, s2)

	// Winkler modification
	prefixLength := commonPrefixLength(s1, s2)

	if prefixLength > maxPrefixLength {
		prefixLength = maxPrefixLength
	}

	return jaroSimilarity + (float32(prefixLength) * 0.1 * (1.0 - jaroSimilarity))
}

func jaroSimilarity(s1, s2 []rune) float32 {
	matchDistance := max(len(s1), len(s2))/2 - 1

	matches := countMatches(s1, s2, matchDistance)
	transpositions := countTranspositions(s1, s2, matches)

	if matches == 0 {
		return 0.0
	}

	return (1.0 / 3.0) * ((float32(matches) / float32(len(s1))) +
		(float32(matches) / float32(len(s2))) +
		(float32(matches-transpositions) / float32(matches)))
}

func countMatches(s1, s2 []rune, matchDistance int) int {
	matches := 0
	matchFlags := big.NewInt(0)

	for i := range s1 {
		start := max(0, i-matchDistance)
		end := min(len(s2)-1, i+matchDistance)

		for j := start; j <= end; j++ {
			if matchFlags.Bit(j) == 0 && s1[i] == s2[j] {
				matchFlags.SetBit(matchFlags, j, 1)
				matches++
				break
			}
		}
	}

	return matches
}

func countTranspositions(s1, s2 []rune, matches int) int {
	transpositions := 0
	j := 0

	for i := range s1 {
		if matches > 0 && j < len(s2) && s1[i] == s2[j] {
			matches--
		}
		if j < len(s2) && s1[i] != s2[j] {
			transpositions++
		}
		if j < len(s2) {
			j++
		}
	}

	return transpositions / 2
}

func commonPrefixLength(s1, s2 []rune) int {
	i := 0
	for i < len(s1) && i < len(s2) && s1[i] == s2[i] {
		i++
	}
	return i
}

/***************************************
 * Levenshtein distance
 ***************************************/

type LevenshteinDistance struct {
	input    string
	inputLen int32
	matrix   [][]int32
}

func numberOfRunes(in string) (n int) {
	for i := range in {
		n = i
	}
	return n + 1
}

func NewLevenshteinDistance(input string) (result LevenshteinDistance) {
	result.input = strings.ToUpper(input)
	result.inputLen = int32(numberOfRunes(result.input))
	result.matrix = make([][]int32, 0, result.inputLen+1)
	return result
}

func (x *LevenshteinDistance) Input() string {
	return string(x.input)
}
func (x *LevenshteinDistance) Score(candidate string) float32 {
	nInput := x.inputLen
	nCandidate := int32(numberOfRunes(candidate))

	// reuse the matrix between each comparaison
	for i := int32(len(x.matrix)); i < nCandidate+1; i++ {
		x.matrix = append(x.matrix, make([]int32, nInput+1))
	}

	// populate the matrix
	for i := int32(0); i <= nCandidate; i++ {
		for j := int32(0); j <= nInput; j++ {
			if i == 0 {
				x.matrix[i][j] = int32(j)
			} else if j == 0 {
				x.matrix[i][j] = int32(i)
			} else {
				break
			}
		}
	}

	for i, ich := range candidate {
		ich = unicode.ToUpper(ich) // make case insensitive

		for j, jch := range x.input {
			cost := int32(0)
			if ich != jch {
				cost = 1
			}

			x.matrix[i+1][j+1] = min(
				x.matrix[i][j+1]+1, // Deletion
				x.matrix[i+1][j]+1, // Insertion
				x.matrix[i][j]+ // Substitution
					cost) // Case insensitive match
		}
	}

	// score is the value in the bottom-right cell of the matrix
	score := x.matrix[nCandidate][nInput]

	// scale the score to diminish further penalties
	score = score * 2

	// adjust the score based on the length difference
	if lengthDifference := nCandidate - nInput; lengthDifference > 0 {
		score -= lengthDifference
	} else {
		score += lengthDifference
	}

	return float32(score)
}

/***************************************
 * Jaro-Winkler similarity with Levenshtein distance for common prefix
 ***************************************/

type JaroWinklerLevenshteinSimilarity struct {
	jaroWinklerSimilarity JaroWinklerSimilarity
	levenshteinDistance   LevenshteinDistance
}

func NewJaroWinklerLevenshteinSimilarity(input string, maxPrefixLength int) JaroWinklerLevenshteinSimilarity {
	return JaroWinklerLevenshteinSimilarity{
		jaroWinklerSimilarity: NewJaroWinklerSimilarity(input, maxPrefixLength),
		levenshteinDistance:   NewLevenshteinDistance(input),
	}
}

func (x JaroWinklerLevenshteinSimilarity) Input() string {
	return x.levenshteinDistance.input
}
func (x JaroWinklerLevenshteinSimilarity) Score(candidate string) float32 {
	s1 := x.jaroWinklerSimilarity.input
	s2 := []rune(strings.ToUpper(candidate))

	// Jaro similarity
	jaroSimilarity := jaroSimilarity(s1, s2)

	// Winkler modification with Levenshtein distance
	prefixLength := int(x.levenshteinDistance.Score(candidate))

	if prefixLength > x.jaroWinklerSimilarity.MaxPrefixLength {
		prefixLength = x.jaroWinklerSimilarity.MaxPrefixLength
	}

	return jaroSimilarity + (float32(prefixLength) * 0.1 * (1.0 - jaroSimilarity))
}

/***************************************
 * AutoComplete
 ***************************************/

type AutoCompleteResult struct {
	Text        string
	Description string
	Score       float32
}

func (x *AutoCompleteResult) Compare(o *AutoCompleteResult) int {
	if x.Score != o.Score {
		if x.Score > o.Score {
			return -1
		}
		return 1
	} else {
		return strings.Compare(x.Text, o.Text)
	}
}
func (x *AutoCompleteResult) String() string {
	return x.Text
}

type AutoComplete interface {
	Input() string
	Any(interface{}) error
	Append(in AutoCompletable)
	Add(in, description string) float32
	Highlight(in string, highlight func(rune) string) string
	Results() []AutoCompleteResult
	ClearResults()
}

type AutoCompletable interface {
	AutoComplete(AutoComplete)
}

type BasicAutoComplete struct {
	entries    []AutoCompleteResult
	maxResults int
	similarity StringSimilarity
}

func NewAutoComplete(input string, maxResults int) BasicAutoComplete {
	return BasicAutoComplete{
		entries:    make([]AutoCompleteResult, 0, maxResults),
		maxResults: maxResults,
		similarity: NewJaroWinklerSimilarity(input, 3),
	}
}
func (x *BasicAutoComplete) Input() string {
	return x.similarity.Input()
}
func (x *BasicAutoComplete) Any(anon interface{}) error {
	if autocomplete, ok := anon.(AutoCompletable); ok {
		autocomplete.AutoComplete(x)
		return nil
	} else {
		err := fmt.Errorf("%T: type does not support auto-complete", anon)
		return err
	}
}
func (x *BasicAutoComplete) Append(in AutoCompletable) {
	in.AutoComplete(x)
}
func (x *BasicAutoComplete) Add(in, description string) float32 {
	newEntry := AutoCompleteResult{
		Text:        in,
		Description: description,
		Score:       x.similarity.Score(in),
	}

	x.entries = AppendBoundedSort(x.entries, x.maxResults, newEntry, func(a, b AutoCompleteResult) bool {
		return a.Compare(&b) < 0
	})

	return newEntry.Score
}
func (x *BasicAutoComplete) Results() []AutoCompleteResult {
	if len(x.entries) > 0 && x.Input() == strings.ToUpper(x.entries[0].Text) {
		x.entries = RemoveUnless(func(acr AutoCompleteResult) bool {
			return strings.HasPrefix(strings.ToUpper(acr.Text), x.Input())
		}, x.entries...)
	}
	return x.entries
}
func (x *BasicAutoComplete) Highlight(in string, highlight func(rune) string) string {
	var highlightedS2 strings.Builder

	s1 := []rune(x.similarity.Input())
	s2 := []rune(strings.ToUpper(in))

	matchDistance := max(len(s1), len(s2))/2 - 1

	matches := 0
	matchFlags := big.NewInt(0)

	for i := range s1 {
		start := max(0, i-matchDistance)
		end := min(len(s2)-1, i+matchDistance)

		for j := start; j <= end; j++ {
			if matchFlags.Bit(j) == 0 && s1[i] == s2[j] {
				matchFlags.SetBit(matchFlags, j, 1)
				matches++
				break
			}
		}
	}

	for i, ch := range in {
		if matchFlags.Bit(i) != 0 {
			highlightedS2.WriteString(highlight(ch))
		} else {
			highlightedS2.WriteRune(ch)
		}
	}

	return highlightedS2.String()
}
func (x *BasicAutoComplete) ClearResults() {
	x.entries = x.entries[:0]
}

/***************************************
 * PrefixedAutoComplete adds a prefix to each possible string
 ***************************************/

type PrefixedAutoComplete struct {
	prefix      string
	description string

	inner AutoComplete
}

func NewPrefixedAutoComplete(prefix, description string, inner AutoComplete) PrefixedAutoComplete {
	return PrefixedAutoComplete{
		prefix:      prefix,
		description: description,
		inner:       inner,
	}
}
func (x *PrefixedAutoComplete) Input() string {
	return x.inner.Input()
}
func (x *PrefixedAutoComplete) Any(anon interface{}) error {
	if autocomplete, ok := anon.(AutoCompletable); ok {
		autocomplete.AutoComplete(x)
		return nil
	} else {
		return fmt.Errorf("%T: type does not support auto-complete", anon)
	}
}
func (x *PrefixedAutoComplete) Append(in AutoCompletable) {
	in.AutoComplete(x)
}
func (x *PrefixedAutoComplete) Add(in, description string) float32 {
	return x.inner.Add(x.prefix+in, fmt.Sprint(x.description, ": ", description))
}
func (x *PrefixedAutoComplete) Results() []AutoCompleteResult {
	return x.inner.Results()
}
func (x *PrefixedAutoComplete) Highlight(in string, highlight func(rune) string) string {
	return x.inner.Highlight(in, highlight)
}
func (x *PrefixedAutoComplete) ClearResults() {
	x.inner.ClearResults()
}

/***************************************
 * XXX not founmd, did you mean YYY?
 ***************************************/

func DidYouMean[T AutoCompletable](in string) (string, error) {
	in = strings.ToLower(in)
	autocomplete := NewAutoComplete(in, 3)

	var defaultValue T
	defaultValue.AutoComplete(&autocomplete)

	results := autocomplete.Results()
	if len(results) > 0 && strings.ToLower(results[0].Text) == in {
		return results[0].Text, nil
	}

	return "", fmt.Errorf("unknown value %q, did you mean %v?", in, autocomplete.Results())
}
