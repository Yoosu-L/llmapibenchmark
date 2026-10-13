package api

import (
	"math/rand"
	"strings"
)

const (
	minWordLength = 3
	maxWordLength = 10
)

var letters = []rune("abcdefghijklmnopqrstuvwxyz")

// generateRandomWord returns a random lowercase letter string. Deliberately
// not natural words: all tokenizers fall back to the same byte-level
// fragments, so the tokens-per-word ratio stays roughly stable across models.
func generateRandomWord() string {
	// Word length ranges from 3 to 10 characters.
	wordLength := minWordLength + rand.Intn(maxWordLength-minWordLength+1)

	word := make([]rune, wordLength)

	for i := 0; i < wordLength; i++ {
		word[i] = letters[rand.Intn(len(letters))]
	}

	return string(word)
}

// generateRandomPhrase builds a cache-busting prompt of numWords random words.
func generateRandomPhrase(numWords int) string {
	randomWords := make([]string, numWords)
	for i := 0; i < numWords; i++ {
		randomWords[i] = generateRandomWord()
	}

	randomPhrase := strings.Join(randomWords, " ")

	result := "Please reply back the following section unchanged: " + randomPhrase

	return result
}
