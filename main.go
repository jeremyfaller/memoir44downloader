package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"gopkg.in/gomail.v2"
)

var (
	urlFlag      = flag.String("url", "https://www.daysofwonder.com/memoir44/en/mini-campaigns/remembrance", "url to download")
	daemonFlag   = flag.Bool("deamon", true, "should this run as a daemon")
	sleepFlag    = flag.Int("sleep", 480, "duration to this sleep in daemon mode")
	toFlag       = flag.String("to", os.Getenv("MEM44_TO_EMAIL"), "comma separated list of email addresses to send to")
	sendMailFlag = flag.Bool("send", true, "should mail be sent")
	smtpAcctFlag = flag.String("from", os.Getenv("MEM44_SMTP_ACCOUNT"), "smtp account holder")
	smtpPassFlag = flag.String("password", os.Getenv("MEM44_SMTP_PASSWORD"), "smtp password")
	smtpHostFlag = flag.String("host", "smtp.gmail.com", "smtp host")
	smtpPortFlag = flag.Int("port", 587, "smtp port")
	hashFileFlag = flag.String("hash", os.Getenv("HOME")+"/.mem44hash", "where to store the last hash")
)

const fullDateFormat = "Jan 2 2006, 15:04:05"

var (
	lastDownloadTime = time.Now()
	lastDownloadHash string
	thisDownloadHash = sha256.New()
)

func subject() string {
	return fmt.Sprintf("Memoir '44 Download from (%s)", time.Now().Format("Jan 2, 2006"))
}

func body() string {
	return fmt.Sprintf(
		`
<html>
<h1>Hello</h1>
<br>
<p>I am your friendly neighborhood Memoir '44 downloader. I download the Rememberance map on the
<a href=%q>Days of Wonder</a> website. I thought you wanted this month's map; therefore, I've
included it.</p>
<p>Here are some stats about me. I hope you find these interesting.</p>
<table>
  <tr>
    <th>lastDownloadTime:</th>
	<th>%s</th>
  </tr>
  <tr>
    <th>lastDownloadHash:</th>
	<th>%s</th>
  </tr>
  <tr>
    <th>thisDownloadHash:</th>
	<th>%x</th>
  </tr>
</table>
</html>
`,
		*urlFlag,
		lastDownloadTime.Format(fullDateFormat),
		lastDownloadHash,
		thisDownloadHash.Sum(nil),
	)
}

func convertHash(h hash.Hash) string {
	return fmt.Sprintf("%x", h.Sum(nil))
}

func main() {
	flag.Parse()
	if len(*urlFlag) == 0 {
		log.Fatal("no -url specified")
	}

	emails := strings.Split(*toFlag, ",")
	sleepDur := time.Duration(*sleepFlag) * time.Minute

	lastHash, _ := ioutil.ReadFile(*hashFileFlag)
	lastDownloadHash = string(lastHash)

	for {
		func() {
			doc, err := goquery.NewDocument(*urlFlag)
			if err != nil {
				log.Fatalf("error getting document: %v\nerr: %v", *urlFlag, err)
			}
			log.Printf("connected to DoW")

			var pdfURL string
			doc.Find("a[href]").Each(func(index int, item *goquery.Selection) {
				if href, _ := item.Attr("href"); strings.HasSuffix(href, ".pdf") {
					pdfURL = href
				}
			})
			if len(pdfURL) == 0 {
				log.Fatalf("no pdf url at %v", *urlFlag)
			}
			log.Printf("downloading PDF")

			resp, err := http.Get(pdfURL)
			if err != nil {
				log.Fatalf("error downloading: %v\nerr: %v", pdfURL, err)
			}
			defer resp.Body.Close()

			tmp, err := ioutil.TempFile("/tmp", "mem44")
			if err != nil {
				log.Fatalf("error opening temp file: %v", err)
			}
			defer os.Remove(tmp.Name())

			thisDownloadHash.Reset()
			teedBody := io.TeeReader(resp.Body, thisDownloadHash)

			sz, err := io.Copy(tmp, teedBody)
			if err != nil {
				log.Fatalf("error saving temp file:  %v", err)
			}
			log.Printf("size: %d", sz)

			if convertHash(thisDownloadHash) == lastDownloadHash {
				log.Printf("hashes are equal, skipping download")
				return
			}

			mesg := gomail.NewMessage()
			mesg.SetHeader("From", *smtpAcctFlag)
			mesg.SetHeader("To", emails...)
			mesg.SetHeader("Subject", subject())
			mesg.SetBody("text/html", body())
			mesg.Attach(tmp.Name())
			d := gomail.NewDialer(*smtpHostFlag, *smtpPortFlag, *smtpAcctFlag, *smtpPassFlag)
			if *sendMailFlag {
				if err := d.DialAndSend(mesg); err != nil {
					log.Fatalf("error sending mail: %v", err)
				}
				log.Printf("mail sent")
			} else {
				log.Printf("mail skipped")
			}

		}()

		if !*daemonFlag {
			break
		}
		lastDownloadHash = convertHash(thisDownloadHash)
		ioutil.WriteFile(*hashFileFlag, []byte(lastDownloadHash), 0600)
		lastDownloadTime = time.Now()
		log.Printf("sleeping till: %s", time.Now().Add(sleepDur).Format(fullDateFormat))
		time.Sleep(sleepDur)
	}
}
