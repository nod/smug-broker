// broker: slack
// provides our interface for consuming from, and publishing, to slack

// NOTE ABOUT CREDENTIALS
// this requires slack credentials to be created for your network and the bot to
// be invited to the specific channel.
// In particular, know that the slack api is wide open unless you go through the
// motions of creating a bot specific access token.  That means private messages
// can flow through this bot from the creating user and replies to that user,
// regardless if the bot is privy to those conversations or not.

package smug

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"sync"
	"time"

	libsl "github.com/slack-go/slack"
)


/* ************************** *
 * fake the slacklib logger
 * ************************** */

type SlackLogger struct {
	*Logger
}

func (sl *SlackLogger) Output(lvl int, msg string) error {
	sl.Info(msg)
	return nil
}

/* ************************** *
 * repr our slack users
 * ************************** */

type SlackUser struct {
	Id     string
	Nick   string
	Avatar string
}

type SlackUserCache struct {
	mux   sync.RWMutex
	users map[string]*SlackUser
	nicks map[string]*SlackUser
}

func (suc *SlackUserCache) CacheUser(user *SlackUser) {
	suc.mux.Lock()
	defer suc.mux.Unlock()
	suc.users[user.Id] = user
	suc.nicks[strings.ToLower(user.Nick)] = user
}

func (suc *SlackUserCache) UserFromAPI(
	sb *SlackBroker, ukey string) (*SlackUser, error) {
	user, err := sb.api.GetUserInfo(ukey)
	if err != nil {
		return nil, fmt.Errorf("err fetching user from slack: %+v", err)
	}
	suser := &SlackUser{
		Id:     ukey,
		Nick:   user.Name,
		Avatar: user.Profile.Image72,
	}
	suc.CacheUser(suser)
	return suser, nil
}

func (suc *SlackUserCache) userInIdCache(ukey string) (*SlackUser, bool) {
	suc.mux.RLock()
	defer suc.mux.RUnlock()
	user, found := suc.users[ukey]
	return user, found
}

func (suc *SlackUserCache) userInNickCache(nick string) (*SlackUser, bool) {
	suc.mux.RLock()
	defer suc.mux.RUnlock()
	user, found := suc.nicks[strings.ToLower(nick)]
	return user, found
}

func (suc *SlackUserCache) UserNick(
	sb *SlackBroker, ukey string, cacheOnly bool) string {
	cached_user, found := suc.userInIdCache(ukey)
	if found {
		return cached_user.Nick
	}
	if cacheOnly {
		return ""
	}
	user, err := suc.UserFromAPI(sb, ukey)
	if err != nil {
		sb.log.Warnf("attempted to fetch %s but got err: %v", ukey, err)
		return ""
	}
	return user.Nick
}

func (suc *SlackUserCache) UserId(
	sb *SlackBroker, nick string, cacheOnly bool) string {
	cached_user, found := suc.userInNickCache(nick)
	if found {
		return cached_user.Id
	}
	if cacheOnly {
		// possibly don't want to do api calls for whatever reason
		return ""
	}
	user, err := suc.UserFromAPI(sb, nick)
	if err != nil {
		sb.log.Warnf("attempted to fetch %s but got err: %v", nick, err)
		return ""
	}
	return user.Id
}

func (suc *SlackUserCache) PopulateCache(sb *SlackBroker, mems []string) {
	for _, uid := range mems {
		suc.UserFromAPI(sb, uid)
	}
}

func (suc *SlackUserCache) Setup() {
	suc.mux.Lock()
	defer suc.mux.Unlock()
	suc.users = make(map[string]*SlackUser)
	suc.nicks = make(map[string]*SlackUser)
}

/* ************************** *
 * slack broker
 * ************************** */

type SlackBroker struct {
	log *Logger
	// components from slack lib
	api *libsl.Client
	rtm *libsl.RTM
	// internal plumbing
	usercache       *SlackUserCache
	chanid          string
	channel         string
	token           string
	mybotid         string
	re_uids         *regexp.Regexp
	re_usernick     *regexp.Regexp
	re_atusers      *regexp.Regexp
	re_embeddedurls *regexp.Regexp
	msgsMux  sync.RWMutex
	msgsSent int64
	msgsRcvd int64
}

func (sb *SlackBroker) Name() string {
	return fmt.Sprintf("slack-%s", sb.channel)
}

func (sb *SlackBroker) Heartbeat() bool {
    sb.msgsMux.Lock()
    mr,ms := sb.msgsRcvd, sb.msgsSent
    sb.msgsRcvd, sb.msgsSent = 0,0
    sb.msgsMux.Unlock()
    sb.log.logMetrics(mr,ms)
    return true
}

// allows us to setup internal members without hitting the api
// let's us do certain tests that don't require api
func (sb *SlackBroker) SetupInternals() {
	sb.log = NewLogger("broker", "slack")
	sb.usercache = &SlackUserCache{}
	sb.usercache.Setup()
	sb.re_uids = regexp.MustCompile(`<@(U[\w|]+)>`) // get sub ids in msgs
	sb.re_usernick = regexp.MustCompile(`^(\w+):`)
	sb.re_atusers = regexp.MustCompile(`@(\w+)\b`)
	sb.re_embeddedurls = regexp.MustCompile(`<(http.+\|?.*)>`)
}

func (sb *SlackBroker) ConvertRefsToUsers(s string, cacheOnly bool) string {
	matches := sb.re_uids.FindAllStringSubmatchIndex(s, -1)
	// will eventually contain a uniq set of uids mentioned
	uids := make(map[string]string) // uidstr->nick
	// first, rip through all patterns and look up in cache
	for i := len(matches) - 1; i >= 0; i-- {
		// start,stop,sub0,sublen := matches[i]
		m := matches[i]
		uid := s[m[2]:m[3]]
		var nick string
		if strings.Contains(uid, "|") {
			parts := strings.Split(uid, "|")
			if len(parts) > 0 {
				nick = sb.usercache.UserNick(sb, parts[0], cacheOnly)
			}
		} else {
			nick = sb.usercache.UserNick(sb, uid, cacheOnly)
		}
		if _, found := uids[uid]; !found {
			uids[uid] = nick
		}
	}
	// now iterate over refs and replace them all
	for u, n := range uids {
		s = strings.ReplaceAll(
			s,
			fmt.Sprintf("<@%s>", u),
			n, //sb.usercache.UserNick(sb, u, cacheOnly),
		)
	}
	return s
}

func (sb *SlackBroker) ConvertUsersToRefs(s string, cacheOnly bool) string {
	//  first look for irc type  USER: at beginning of line
	matches := sb.re_usernick.FindAllStringSubmatchIndex(s, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		// start,stop,sub0,sublen := matches[i]
		m := matches[i]
		usernick := s[m[2]:m[3]]
		uid := sb.usercache.UserId(sb, usernick, cacheOnly)
		if len(uid) > 4 {
			s = strings.ReplaceAll(
				s,
				usernick,
				fmt.Sprintf("@%s", uid),
			)
		}
	}

	//  then do embedded @user replacements
	matches = sb.re_atusers.FindAllStringSubmatchIndex(s, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		// start,stop,sub0,sublen := matches[i]
		m := matches[i]
		usernick := s[m[2]:m[3]]
		uid := sb.usercache.UserId(sb, usernick, cacheOnly)
		if len(uid) > 1 {
			s = strings.ReplaceAll(
				s,
				"@"+usernick,
				fmt.Sprintf("<@%s>", uid),
			)
		}
	}

	return s
}

// args [token, channel]
func (sb *SlackBroker) Setup(args ...string) {
	sb.SetupInternals()
	sb.token = args[0]
	sb.channel = args[1]
	if strings.HasPrefix(sb.channel, "#") {
		sb.log.Warnf("slack channels should not begin with #")
	}
	sc := libsl.New(
		sb.token,
		libsl.OptionDebug(false),
		// libsl.OptionLog(&SlackLogger{sb.log}),
	)
	sb.api = sc
	sb.rtm = sb.api.NewRTM()
	authtest, err := sb.api.AuthTest() // gets our identity from slack api
	myuid := authtest.UserID
	if err != nil {
		sb.log.Warnf("ERR occurred %+v", err)
	}
	sb.mybotid = myuid

	// populate my channel info
	// this is a bit ... lame. Should be better way?  XXX
	channels, _ := sb.api.GetChannels(false)
	for _, channel := range channels {
		if channel.Name == sb.channel {
			sb.chanid = channel.ID
			sb.usercache.PopulateCache(sb, channel.Members)
			break
		}
	}
	if sb.chanid == "" {
		sb.log.Warnf("ERR channel not found (%s)", sb.channel)
		return
	}
}

func (sb *SlackBroker) SendComplexMsg(dest string, text string, ev *Event) {

}

func (sb *SlackBroker) HandleEvent(ev *Event, dis Dispatcher) {
	if ev.ReplyBroker != nil && ev.ReplyBroker != sb {
		// if not intended for us, eject here
		return
	}
    sb.msgsMux.Lock()
    sb.msgsRcvd++
    sb.msgsMux.Unlock()
	txt := sb.ConvertUsersToRefs(ev.Text, false)
	var dest string
	if len(ev.ReplyTarget) == 0 {
		dest = sb.chanid
	} else {
		dest = ev.ReplyTarget
	}

	var msgContent libsl.MsgOption
	if ev.ContentBlocks != nil && len(ev.ContentBlocks) > 0 {
		blockslice := []libsl.Block{}
		for _, db := range ev.ContentBlocks {
			var headerText *libsl.TextBlockObject
			var headerSect *libsl.SectionBlock
			if len(db.Title) > 0 {
				// just bold it
				headerText = libsl.NewTextBlockObject(
					"mrkdwn", "*"+db.Title+"*", false, false)
				headerSect = libsl.NewSectionBlock(
					headerText, nil, nil)
				blockslice = append(blockslice, headerSect)
			}
			// continue if nothing else but header in this block
			if db.Text == "" && db.ImgUrl == "" {
				continue
			}
			msgText := libsl.NewTextBlockObject("mrkdwn", db.Text, false, false)
			if len(db.ImgUrl) > 0 {
				msgImg := libsl.NewImageBlockElement(db.ImgUrl, "accimg")
				msgSect := libsl.NewSectionBlock(
					msgText, nil, libsl.NewAccessory(msgImg))
				blockslice = append(blockslice, msgSect)
			} else {
				msgSect := libsl.NewSectionBlock(
					msgText, nil, nil)
				blockslice = append(blockslice, msgSect)
			}
		}
		msgContent = libsl.MsgOptionBlocks(blockslice...)
	} else {
		msgContent = libsl.MsgOptionText(txt, false)
	}
	sb.api.PostMessage(
		dest,
		libsl.MsgOptionText("", false),
		msgContent,
		libsl.MsgOptionUsername(ev.Actor),
		libsl.MsgOptionIconEmoji(fmt.Sprintf(":avatar_%s:", ev.Actor)),
	)
}

// accept a slack string and simplify it
// - replace html entities (&lt; should be <)
// - remove urls in favor of url descr where available
func (sb *SlackBroker) SimplifyParse(s string) string {
	matches := sb.re_embeddedurls.FindAllStringSubmatchIndex(s, -1)
	// start at the end for replacement, this is a bit janky. XXX
	for i := len(matches) - 1; i >= 0; i-- {
		// start,stop,sub0,sublen := matches[i]
		m := matches[i]
		entire_url := s[m[0]:m[1]]
		parts := strings.Split(s[m[2]:m[3]], "|")
		var udescr string
		if len(parts) > 2 || len(parts) == 1 {
			// <http> or <?????>
			udescr = parts[0]
		} else if len(parts) == 2 {
			// <http|descr> or <http|>
			if len(parts[1]) > 0 {
				udescr = parts[1]
			} else {
				udescr = parts[0]
			}
		} else {
			// something screwy man
			udescr = entire_url
		}
		s = strings.ReplaceAll(s, entire_url, udescr)
	}
	return html.UnescapeString(s)
}

func (sb *SlackBroker) ParseToEvent(e *libsl.MessageEvent) *Event {
	nick := sb.usercache.UserNick(sb, e.User, false)
	outmsgs := []string{e.Text}
	if len(e.Files) > 0 {
		for _, f := range e.Files {
			outmsgs = append(outmsgs,
				fmt.Sprintf("%s(%s)", f.Name, f.URLPrivate))
		}
	}
	if len(e.Attachments) > 0 {
		for _, a := range e.Attachments {
			if len(a.Fallback) > 0 {
				outmsgs = append(outmsgs, a.Fallback)
			}
			/*
			   if len(a.Text) > 0 {
			       outmsgs = append(outmsgs, a.Text)
			   }
			*/
			if len(a.ImageURL) > 0 {
				outmsgs = append(outmsgs,
					fmt.Sprintf("%s %s", a.Title, a.ImageURL))
			}
		}
	}
	// XXX TODO need to include the RespondTo stuff if priv msg...
	outstr := strings.TrimSpace(strings.Join(outmsgs, " "))
	ev := &Event{
		Origin:  sb,
		Actor:   nick,
		RawText: outstr,
		Text:    sb.SimplifyParse(sb.ConvertRefsToUsers(outstr, false)),
		ts:      time.Now(),
	}
	return ev
}

func (sb *SlackBroker) Activate(dis Dispatcher) {
	if sb.rtm == nil {
		// raise some error here XXX TODO
		sb.log.Panic(fmt.Errorf("rtm is nil.  Setup not called?"))
	}
	go sb.rtm.ManageConnection()
	for msg := range sb.rtm.IncomingEvents {
		switch e := msg.Data.(type) {
		case *libsl.HelloEvent:
			// ignore Hello
		case *libsl.UserTypingEvent:
			// ignore typing
		case *libsl.ConnectedEvent:
			sb.log.Infof("joining chan: %s", sb.channel)
		case *libsl.MessageEvent:
			// smugbot: 2019/09/14 08:47:44 websocket_managed_conn.go:369:
			// Incoming Event:
			// {"client_msg_id":"ed722fbc-5b37-4f78-9981-e3c9ce5c85a1","suppress_notification":false,"type":"message","text":"test","user":"U6CRHMXK4","team":"T6CRHMX5G","user_team":"T6CRHMX5G","source_team":"T6CRHMX5G","channel":"C6MR9CBGR","event_ts":"1568468854.004200","ts":"1568468854.004200"}
			if e.BotID != sb.mybotid && len(e.User) > 0 {
				ev := sb.ParseToEvent(e)
				if e.Channel != sb.chanid {
					// possibly from a private message or other non-channel
					ev.ReplyBroker = sb
					ev.ReplyTarget = e.Channel
				}
                sb.msgsMux.Lock()
                sb.msgsSent++
                sb.msgsMux.Unlock()
				dis.Broadcast(ev)
			}
		case *libsl.PresenceChangeEvent:
			sb.log.Infof("Presence Change: %v\n", e)
		case *libsl.LatencyReport:
			sb.log.Infof("Current latency: %v\n", e.Value)
		case *libsl.RTMError:
			sb.log.Warnf("Error: %s\n", e.Error())
		case *libsl.InvalidAuthEvent:
			sb.log.Fatalf("Invalid credentials")
			return
		default:
			// Ignore other events..
			sb.log.Infof("Unexpected: %v\n", msg.Data)
		}
	}
}

func (sb *SlackBroker) Deactivate() {}
