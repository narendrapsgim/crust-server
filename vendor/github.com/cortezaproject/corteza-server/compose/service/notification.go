package service

import (
	"context"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	gomail "gopkg.in/mail.v2"

	httpClient "github.com/cortezaproject/corteza-server/pkg/http"
	"github.com/cortezaproject/corteza-server/pkg/logger"
	"github.com/cortezaproject/corteza-server/pkg/mail"
	"github.com/cortezaproject/corteza-server/system/types"
)

type (
	notification struct {
		logger *zap.Logger
		users  notificationUserFinder
	}

	notificationUserFinder interface {
		FindByID(context.Context, uint64) (*types.User, error)
	}
)

func Notification() *notification {
	return &notification{
		logger: DefaultLogger.Named("notification"),
		users:  DefaultSystemUser,
	}
}

// log() returns zap's logger with requestID from current context and fields.
func (svc notification) log(ctx context.Context, fields ...zapcore.Field) *zap.Logger {
	return logger.AddRequestID(ctx, svc.logger).With(fields...)
}

func (svc notification) SendEmail(ctx context.Context, message *gomail.Message) error {
	return mail.Send(message)
}

// AttachEmailRecipients validates, resolves, formats and attaches set of recipients to message
//
// Supports 3 input formats:
//  - <valid email>
//  - <valid email><space><name...>
//  - <userID>
// Last one is then translated into valid email + name (when/if possible)
func (svc notification) AttachEmailRecipients(ctx context.Context, message *gomail.Message, field string, recipients ...string) (err error) {
	var (
		email string
		name  string
	)

	if len(recipients) == 0 {
		return
	}

	for r, rcpt := range recipients {
		name, email = "", ""
		rcpt = strings.TrimSpace(rcpt)

		if userID, err := strconv.ParseUint(rcpt, 10, 64); err == nil && userID > 0 {
			// handle user ID
			if user, err := svc.users.FindByID(ctx, userID); err != nil {
				return errors.Wrap(err, "could not get notification address")
			} else {
				email = user.Email
				name = user.Name
			}

		} else if spaceAt := strings.Index(rcpt, " "); spaceAt > -1 {
			// handle: <email> <name> ("foo@bar.baz foo baz")
			email, name = rcpt[:spaceAt], strings.TrimSpace(rcpt[spaceAt+1:])
		} else {
			// handle: <email>
			email = rcpt
		}

		// Validate email here
		if !mail.IsValidAddress(email) {
			return errors.New("Invalid recipient email format")
		}

		recipients[r] = message.FormatAddress(email, name)
	}

	message.SetHeader(field, recipients...)
	return
}

func (svc notification) AttachRemoteFiles(ctx context.Context, message *gomail.Message, rr ...string) error {
	var (
		wg = &sync.WaitGroup{}
		l  = &sync.Mutex{}

		client, err = httpClient.New(&httpClient.Config{
			Timeout: 10,
		})

		log = svc.logger
	)

	log.Debug("attaching files to mail notification", zap.Strings("urls", rr))

	if err != nil {
		return errors.WithStack(err)
	}

	get := func(log *zap.Logger, req *http.Request) {
		defer wg.Done()

		resp, err := client.Do(req)
		if err != nil {
			log.Error("could not send request to download remote attachment", zap.Error(err))
			return
		}

		if resp.StatusCode != http.StatusOK {
			log.Error("could not download remote attachment", zap.String("status", resp.Status))
			return
		}

		log.Info("download successful",
			zap.Int64("content-length", resp.ContentLength),
			zap.String("content-type", resp.Header.Get("Content-Type")),
		)

		l.Lock()
		defer l.Unlock()
		message.AttachReader(path.Base(req.URL.Path), resp.Body)
	}

	for _, url := range rr {
		log := log.With(zap.String("remote-file", url))

		req, err := client.Get(url)
		if err != nil {
			return errors.WithStack(err)
		}

		wg.Add(1)
		go get(log, req)
	}

	wg.Wait()
	return nil
}
