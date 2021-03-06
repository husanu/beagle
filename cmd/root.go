// Package cmd exports a function to create a cobra command.
package cmd

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/danielkvist/beagle/client"

	"github.com/spf13/cobra"
)

type site struct {
	name    string
	mainURL string
	userURL string
}

// Root returns a *cobra.Command.
func Root() *cobra.Command {
	var (
		agent      string
		debug      bool
		file       string
		goroutines int
		proxy      string
		timeout    time.Duration
		user       string
		verbose    bool
	)

	root := &cobra.Command{
		Use:     "beagle",
		Short:   "Beagle is a CLI written in Go to search for an specific username across the Internet.",
		Example: "beagle -g 10 -t 1s -u me -v",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New(client.WithTimeout(timeout), client.WithProxy(proxy))
			if err != nil {
				return fmt.Errorf("while creating a new http.Client: %v", err)
			}

			f, err := os.Open(file)
			if err != nil {
				return fmt.Errorf("while opening file %q: %v", file, err)
			}

			r := csv.NewReader(bufio.NewReader(f))
			sites, err := readAndParseCSV(r, user)
			if err != nil {
				return fmt.Errorf("while reading file %q: %v", file, err)
			}

			if len(sites) == 0 {
				return fmt.Errorf("csv file %q is empty or is not valid", file)
			}

			results := make(chan string, goroutines)
			sema := make(chan struct{}, goroutines)
			var wg sync.WaitGroup

			defer close(results)
			defer close(sema)

			go func() {
				for msg := range results {
					log.Println(msg)
				}
			}()

			disclaimer()
			for _, s := range sites {
				wg.Add(1)
				sema <- struct{}{}

				go check(s, c, agent, debug, verbose, results, sema, &wg)
			}

			wg.Wait()
			return nil
		},
		SilenceUsage: true,
	}

	root.Flags().StringVarP(&agent, "agent", "a", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:67.0) Gecko/20100101 Firefox/67.0", "user agent")
	root.Flags().BoolVar(&debug, "debug", false, "prints errors messages")
	root.Flags().StringVarP(&file, "file", "f", "./urls.csv", ".csv file with the URLs to check")
	root.Flags().IntVarP(&goroutines, "goroutines", "g", 1, "number of goroutines")
	root.Flags().StringVarP(&proxy, "proxy", "p", "", "proxy URL")
	root.Flags().DurationVarP(&timeout, "timeout", "t", 3*time.Second, "max time to wait for a response from a site")
	root.Flags().StringVarP(&user, "user", "u", "me", "username you want to search for")
	root.Flags().BoolVarP(&verbose, "verbose", "v", false, "prints all the results")

	return root
}

func check(site *site, c *http.Client, agent string, debug bool, verbose bool, out chan<- string, sema <-chan struct{}, wg *sync.WaitGroup) {
	defer func() {
		<-sema
		wg.Done()
	}()

	statusCode, err := makeRequest(c, site.userURL, agent)
	if err != nil && debug {
		out <- err.Error()
		return
	}

	if statusCode != http.StatusOK {
		if verbose {
			out <- fmt.Sprintf("[-] %s NOT FOUND", site.mainURL)
		}
		return
	}

	out <- fmt.Sprintf("[+] %s", site.mainURL)
}

func disclaimer() {
	beagle := `	    __
 \,--------/_/'--o  	Use beagle with
 /_    ___    /~"   	responsibility.
  /_/_/  /_/_/
^^^^^^^^^^^^^^^^^^
`

	fmt.Println(beagle)
}

func readAndParseCSV(r *csv.Reader, user string) ([]*site, error) {
	sites := []*site{}
	for {
		line, err := r.Read()
		if err == io.EOF {
			break
		}

		if len(line) != 3 {
			return nil, fmt.Errorf("line %v has wrong number of fields", line)
		}

		if err != nil {
			return nil, err
		}

		sites = append(sites, &site{
			name:    line[0],
			mainURL: replaceURL(line[1], user),
			userURL: replaceURL(line[2], user),
		})
	}

	return sites, nil
}

func makeRequest(c *http.Client, url string, agent string) (int, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", agent)

	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}

	return resp.StatusCode, nil
}

func replaceURL(s, new string) string {
	return strings.Replace(s, "$", new, 1)
}
