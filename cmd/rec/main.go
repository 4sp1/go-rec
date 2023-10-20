package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
)

type model struct {
	choices []string
	cursor  int
	choice  chan int
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) View() string {
	var b strings.Builder
	for i, choice := range m.choices {
		c := " "
		if m.cursor == i {
			c = "x"
		}
		b.WriteString(c)
		b.WriteString(" ")
		b.WriteString(choice)
		b.WriteString("\n")
	}
	return b.String()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter", " ":
			m.choice <- m.cursor
			return m, tea.Quit
		}
	}
	return m, nil
}

func newModel(choices []string) (model, chan int) {
	sig := make(chan int, 1)
	return model{
		choices: choices,
		cursor:  0,
		choice:  sig,
	}, sig
}

func (m model) Choice() chan int {
	return m.choice
}

// ## OS: ARGS
//
// no arguments
// * start with select audio
// * then select video
// (maybe invert two last steps order's)
//
// ## TARGET DISCLAIMER
//
// only macos 140 no problem I guess to port this
// we can very well rely on the fact go is possible to
// easy cross compilable for other targets
//
//go:generate stringer -type=sectionType
type sectionType int

const (
	sectionStart = sectionType(iota)
	sectionVideo
	sectionAudio
)

type sectionItem struct {
	i int
	x string
}

func (i sectionItem) String() string {
	return fmt.Sprintf("%d > %s", i.i, i.x)
}

type sectionCollection []sectionItem

func (c sectionCollection) choices() []string {
	l := make([]string, len(c))
	for k, i := range c {
		l[k] = i.String()
	}
	return l
}

const (
	ffmpegAvFoundation = "avfoundation"
	ffmpegListDevices  = "-list_devices"
)

func main() {

	// ffmpeg -f avfoundation -list_devices true -i ""

	if err := func() error {
		cmd := exec.Command(
			"ffmpeg", "-f", ffmpegAvFoundation, ffmpegListDevices, "true", "-i", "\"\"")
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		// will exit with error,
		//
		// but we scan stdout
		if err := cmd.Run(); err != nil {
			fmt.Println("cmd: run ffmpeg list_devices:", err)
		}
		xa, ia := []string{}, []int{}
		xv, iv := []string{}, []int{}
		{
			s := bufio.NewScanner(&out)
			var section = sectionStart
			for s.Scan() {
				txt := s.Text()
				if strings.Contains(txt, "AVFoundation indev @") {
					if strings.Contains(txt, "video devices:") {
						section = sectionVideo
						fmt.Println("start section video")
						continue
					}
					if strings.Contains(s.Text(), "audio devices:") {
						section = sectionAudio
						fmt.Println("start section audio")
						continue
					}
					fmt.Println("section", section)
					{
						r := regexp.MustCompile(`\[(\d)\] (.*)`)
						sub := r.FindStringSubmatch(s.Text())
						iStr, x := sub[1], sub[2]
						i, err := strconv.Atoi(iStr)
						if err != nil {
							return fmt.Errorf("strconv: atoi: %w", err)
						}
						switch section {
						case sectionVideo:
							xv = append(xv, x)
							iv = append(iv, i)
						case sectionAudio:
							xa = append(xa, x)
							ia = append(ia, i)
						}
					}
					fmt.Println("scan:", s.Text(), section)
				}
			}
		}
		fmt.Println("len(xa) =", len(xa))
		audioChoices := make(sectionCollection, len(xa))
		for i := range xa {
			audioChoices[i] = sectionItem{
				i: ia[i], x: xa[i],
			}
		}
		videoChoices := make(sectionCollection, len(xv))
		for i := range xv {
			videoChoices[i] = sectionItem{
				i: iv[i], x: xv[i],
			}
		}
		for _, c := range audioChoices {
			fmt.Println("audio", c)
		}
		{
			var a, v int
			{
				// ask audio device
				m, s := newModel(audioChoices.choices())
				p := tea.NewProgram(m)
				if _, err := p.Run(); err != nil {
					return err
				}
				a = <-s
				fmt.Println("audio choice:", a)
			}
			{
				// ask video device
				m, s := newModel(videoChoices.choices())
				p := tea.NewProgram(m)
				if _, err := p.Run(); err != nil {
					return err
				}
				v = <-s
				fmt.Println("video choice:", v)
			}

			// ffmpeg -f avfoundation -i "<screen device index>:<audio device index>" output.mkv
			{
				dev := fmt.Sprintf("%d:%d", v, a)
				out := fmt.Sprintf("recout-%s.mkv", uuid.New().String())
				fmt.Printf("rec: info: dev=%q,out=%q\n", dev, out)
				cmd := exec.Command("ffmpeg", "-f", ffmpegAvFoundation, "-i", dev, out)
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("cmd: run: ffmpeg rec dev=%q out=%q: %w", dev, out, err)
				}
			}

		}

		return nil
	}(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
