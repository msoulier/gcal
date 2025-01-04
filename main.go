package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
    "flag"
    "errors"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
    "github.com/op/go-logging"
)

var (
    log   *logging.Logger = nil
    debug bool = false
    duration string
)

func init() {
    flag.BoolVar(&debug, "debug", false, "Debug logging")
    flag.StringVar(&duration, "duration", "1d", "Duration from now to check (1d|1w)")
    flag.Parse()
    format := logging.MustStringFormatter(
        `%{time:2006-01-02 15:04:05.000-0700} %{level} [%{shortfile}] %{message}`,
    )
    stderrBackend := logging.NewLogBackend(os.Stderr, "", 0)
    stderrFormatter := logging.NewBackendFormatter(stderrBackend, format)
    stderrBackendLevelled := logging.AddModuleLevel(stderrFormatter)
    logging.SetBackend(stderrBackendLevelled)
    if debug {
        stderrBackendLevelled.SetLevel(logging.DEBUG, "gcal")
    } else {
        stderrBackendLevelled.SetLevel(logging.INFO, "gcal")
    }
    log = logging.MustGetLogger("gcal")
}

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	log.Infof("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	log.Debugf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func getEvents(srv *calendar.Service, calid, caldesc string) ([]*calendar.Event, error) {
    events2return := make([]*calendar.Event, 0)
	now := time.Now().Local()
    midnight_today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).
        Format(time.RFC3339)
    midnight_tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.Local).
        Format(time.RFC3339)
    midnight_oneweek := time.Date(now.Year(), now.Month(), now.Day()+7, 0, 0, 0, 0, time.Local).
        Format(time.RFC3339)

    endtime := midnight_tomorrow
    if duration == "1w" {
        endtime = midnight_oneweek
    } else if duration != "1d" {
        log.Errorf("Invalid duration: %s", duration)
        return events2return, errors.New("Invalid duration")
    }

    log.Debugf("Querying calendar %s for events from %s to %s\n", calid, midnight_today, midnight_tomorrow)
	events, err := srv.Events.List(calid).ShowDeleted(false).
		SingleEvents(true).TimeMin(midnight_today).TimeMax(endtime).OrderBy("startTime").Do()
	if err != nil {
		log.Fatalf("Unable to retrieve today's events from calendar %s: %v", calid, err)
        return events2return, err
	}
    if caldesc == "" {
        log.Debug("calendar description is empty, using calendar id")
        caldesc = calid
    }
	log.Debugf("Upcoming events from calendar, duration %s, \"%s\":", duration, caldesc)
	if len(events.Items) == 0 {
		log.Debug("No upcoming events found.")
	} else {
        log.Debugf("Found %d events", len(events.Items))
        events2return = events.Items
	}
    return events2return, nil
}

func getCalendarList(srv *calendar.Service) (*calendar.CalendarList, error) {
    calendar_list, err := srv.CalendarList.List().Do()
    if err != nil {
        return nil, err
    }
    return calendar_list, nil
}

func main() {
	ctx := context.Background()
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, calendar.CalendarReadonlyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)

	srv, err := calendar.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Calendar client: %v", err)
	}

    calendar_list, err := getCalendarList(srv)
    if err != nil {
        panic(err)
    }
    for _, item := range calendar_list.Items {
        events, err := getEvents(srv, item.Id, item.Description)
        if err != nil {
            log.Errorf("%s", err)
            os.Exit(1)
        }
		for _, item := range events {
			date := item.Start.DateTime
			if date == "" {
				date = item.Start.Date
			}
            fmt.Printf("%25s %-55s\n", date, item.Summary)
		}
    }
    os.Exit(0)
}
