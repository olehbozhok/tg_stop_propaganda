package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"gorm.io/gorm/clause"
)

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func getVal(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("not found env %s", key)
	}
	return value
}

func main() {
	// loads values from .env into the system
	_ = godotenv.Load(".env")

	db, err := initDB(
		getVal("DB_USER"), getVal("DB_PASS"), getVal("DB_HOST"), getVal("DB_DBNAME"),
	)
	checkErr(err)

	adminUsername := strings.TrimSpace(getVal("TG_ADMIN"))

	bot, err := tgbotapi.NewBotAPI(getVal("TG_TOKEN"))
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
	defer bot.StopReceivingUpdates()

	urls := []PropagandaURL{}
	atomURLS := sync.Mutex{}

	go func() {
		ticker := time.NewTicker(time.Second * 1)
		for range ticker.C {
			atomURLS.Lock()
			oldURLS := urls
			urls = []PropagandaURL{}
			atomURLS.Unlock()

			if len(oldURLS) == 0 {
				continue
			}

			if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&oldURLS).Error; err != nil {
				log.Println("err insert urls,", err)
			}
		}
	}()

	handleMsg := func(msgUpdate *tgbotapi.Message) {

		switch msgUpdate.CommandWithAt() {
		case "start":
			msg := tgbotapi.NewMessage(msgUpdate.Chat.ID, `Вітаю в чатботі по боротьбі с ТікТок пропаганду. Відправляй сюди посилання на кожне відео з пропагандою та фейковою інформацією, ми будемо їх блокувати`)
			go bot.Send(msg)
			return
		case "download":
			if msgUpdate.From == nil || msgUpdate.From.UserName != adminUsername {
				msg := tgbotapi.NewMessage(msgUpdate.Chat.ID, `Ви не адміністратор`)
				msg.ReplyToMessageID = msgUpdate.MessageID
				bot.Send(msg)
				return
			}
			go func() {
				msg := tgbotapi.NewMessage(msgUpdate.Chat.ID, `Почитаю збирати дані, очікуйте, це може зайняти час, не натискайте команду повторно`)
				msg.ReplyToMessageID = msgUpdate.MessageID
				bot.Send(msg)

				resultURLs := []PropagandaURL{}
				if err := db.Where("is_sent = False").Find(&resultURLs).Error; err != nil {
					msg := tgbotapi.NewMessage(msgUpdate.Chat.ID, `Під час обробки сталась помилка:`+err.Error())
					msg.ReplyToMessageID = msgUpdate.MessageID
					bot.Send(msg)

					log.Println("err during get is_sent = False from db, err: ", err)
					return
				}
				if len(resultURLs) == 0 {
					msg := tgbotapi.NewMessage(msgUpdate.Chat.ID, `Немає нових записів`)
					msg.ReplyToMessageID = msgUpdate.MessageID
					bot.Send(msg)
					return
				}

				buffBytes := bytes.NewBuffer(nil)

				min, max := resultURLs[0].ID, resultURLs[len(resultURLs)-1].ID

				w := csv.NewWriter(buffBytes)
				w.Write([]string{"url"})
				for _, el := range resultURLs {
					err := w.Write([]string{el.URL})
					if err != nil {
						msg := tgbotapi.NewMessage(msgUpdate.Chat.ID, `Під час генерування csv сталась помилка:`+err.Error())
						msg.ReplyToMessageID = msgUpdate.MessageID
						bot.Send(msg)
						log.Println("err during generate csv, err: ", err)
						return
					}

				}

				w.Flush()
				if err := w.Error(); err != nil {
					msg := tgbotapi.NewMessage(msgUpdate.Chat.ID, `Під час очищення csv сталась помилка:`+err.Error())
					msg.ReplyToMessageID = msgUpdate.MessageID
					bot.Send(msg)
					log.Println("err during flush csv, err: ", err)
				}

				doc := tgbotapi.NewDocument(msgUpdate.Chat.ID, tgbotapi.FileReader{
					Name:   fmt.Sprintf("urls_%d-%d.csv", min, max),
					Reader: buffBytes,
				})
				bot.Send(doc)

				db.Model(PropagandaURL{}).Where("id <= ? and id >= ?", max, min).Updates(PropagandaURL{IsSent: true})

				msg = tgbotapi.NewMessage(msgUpdate.Chat.ID, `Готово`)
				msg.ReplyToMessageID = msgUpdate.MessageID
				bot.Send(msg)
			}()
			return
		}

		// handle msg
		msgText := strings.TrimSpace(msgUpdate.Text)
		urlParsed, err := url.Parse(msgText)
		if err != nil || !strings.Contains(msgText, "tiktok.com") {
			msg := tgbotapi.NewMessage(msgUpdate.Chat.ID, `Надішліть, будь ласка, саме посилання на tiktok відео`)
			msg.ReplyToMessageID = msgUpdate.MessageID

			go bot.Send(msg)
			return
		}
		urlParsed.Scheme = "https"

		atomURLS.Lock()
		urls = append(urls, PropagandaURL{URL: urlParsed.String()})
		atomURLS.Unlock()

		msg := tgbotapi.NewMessage(msgUpdate.Chat.ID, `Додано, дякую!`)
		msg.ReplyToMessageID = msgUpdate.MessageID
		go bot.Send(msg)
	}

	wg := sync.WaitGroup{}
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for update := range updates {

				if update.Message != nil {
					// If we got a message
					handleMsg(update.Message)
				}
			}

		}()
	}

	wg.Wait()

}
