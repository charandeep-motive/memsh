package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// confirmAction prompts until the user answers yes or no, defaulting to yes
// on an empty response.
func confirmAction(input io.Reader, output io.Writer, prompt string) (bool, error) {
	reader := bufio.NewReader(input)

	for {
		fmt.Fprint(output, prompt)
		answer, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, fmt.Errorf("read confirmation: %w", err)
		}

		answer = strings.TrimSpace(strings.ToLower(answer))
		switch answer {
		case "", "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}

		if errors.Is(err, io.EOF) {
			return false, nil
		}

		fmt.Fprintln(output, "Please answer Y or n.")
	}
}
