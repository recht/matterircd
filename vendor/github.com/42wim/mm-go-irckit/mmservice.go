package irckit

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mattermost/platform/model"
)

type CommandHandler interface {
	handle(u *User, c *Command, args []string)
}

type Command struct {
	handler func(u *User, toUser *User, args []string)
	params  int
	login   bool
}

func logout(u *User, toUser *User, args []string) {
	u.logoutFromMattermost()
}

func login(u *User, toUser *User, args []string) {
	if u.mc != nil {
		err := u.logoutFromMattermost()
		if err != nil {
			u.MsgUser(toUser, err.Error())
		}
	}
	cred := &MmCredentials{}
	datalen := 4
	if u.Cfg.DefaultTeam != "" {
		cred.Team = u.Cfg.DefaultTeam
		datalen--
	}
	if u.Cfg.DefaultServer != "" {
		cred.Server = u.Cfg.DefaultServer
		datalen--
	}
	if len(args) == datalen {
		cred.Pass = args[len(args)-1]
		cred.Login = args[len(args)-2]
		// no default server or team specified
		if cred.Server == "" && cred.Team == "" {
			cred.Server = args[len(args)-4]
		}
		if cred.Team == "" {
			cred.Team = args[len(args)-3]
		}
		if cred.Server == "" {
			cred.Server = args[len(args)-3]
		}

	}

	// incorrect arguments
	if len(args) != datalen {
		// no server or team
		if cred.Team != "" && cred.Server != "" {
			u.MsgUser(toUser, "need LOGIN <login> <pass>")
			return
		}
		// server missing
		if cred.Team != "" {
			u.MsgUser(toUser, "need LOGIN <server> <login> <pass>")
			return
		}
		// team missing
		if cred.Server != "" {
			u.MsgUser(toUser, "need LOGIN <team> <login> <pass>")
			return
		}
		u.MsgUser(toUser, "need LOGIN <server> <team> <login> <pass>")
		return
	}

	if !u.isValidMMServer(cred.Server) {
		u.MsgUser(toUser, "not allowed to connect to "+cred.Server)
		return
	}

	u.Credentials = cred
	var err error
	u.mc, err = u.loginToMattermost()
	if err != nil {
		u.MsgUser(toUser, err.Error())
		return
	}
	u.addUsersToChannels()
	go u.mc.StatusLoop()
	u.MsgUser(toUser, "login OK")

}

func search(u *User, toUser *User, args []string) {
	postlist := u.mc.SearchPosts(strings.Join(args, " "))
	if postlist == nil || len(postlist.Order) == 0 {
		u.MsgUser(toUser, "no results")
		return
	}
	for i := len(postlist.Order) - 1; i >= 0; i-- {
		timestamp := time.Unix(postlist.Posts[postlist.Order[i]].CreateAt/1000, 0).Format("January 02, 2006 15:04")
		channelname := u.mc.GetChannelName(postlist.Posts[postlist.Order[i]].ChannelId)
		u.MsgUser(toUser, "#"+channelname+" <"+u.mc.GetUser(postlist.Posts[postlist.Order[i]].UserId).Username+"> "+timestamp)
		u.MsgUser(toUser, strings.Repeat("=", len("#"+channelname+" <"+u.mc.GetUser(postlist.Posts[postlist.Order[i]].UserId).Username+"> "+timestamp)))
		for _, post := range strings.Split(postlist.Posts[postlist.Order[i]].Message, "\n") {
			u.MsgUser(toUser, post)
		}
		u.MsgUser(toUser, "")
		u.MsgUser(toUser, "")
	}
}

func searchUsers(u *User, toUser *User, args []string) {
	res, err := u.mc.Client.SearchUsers(model.UserSearch{Term: strings.Join(args, " ")})
	if err != nil {
		u.MsgUser(toUser, fmt.Sprint("Error", err))
		return
	}
	users := res.Data.([]*model.User)
	for _, user := range users {
		u.MsgUser(toUser, fmt.Sprint(user.Nickname, user.FirstName, user.LastName))
	}
}

func scrollback(u *User, toUser *User, args []string) {
	if len(args) != 2 {
		u.MsgUser(toUser, "need SCROLLBACK <channel> <lines>")
		u.MsgUser(toUser, "e.g. SCROLLBACK #bugs 10 (show last 10 lines from #bugs)")
		return
	}
	limit, err := strconv.Atoi(args[1])
	if err != nil {
		u.MsgUser(toUser, "need SCROLLBACK <channel> <lines>")
		u.MsgUser(toUser, "e.g. SCROLLBACK #bugs 10 (show last 10 lines from #bugs)")
		return
	}
	if !strings.Contains(args[0], "#") {
		u.MsgUser(toUser, "need SCROLLBACK <channel> <lines>")
		u.MsgUser(toUser, "e.g. SCROLLBACK #bugs 10 (show last 10 lines from #bugs)")
		return
	}
	args[0] = strings.Replace(args[0], "#", "", -1)
	postlist := u.mc.GetPosts(u.mc.GetChannelId(args[0], u.mc.Team.Id), limit)
	if postlist == nil || len(postlist.Order) == 0 {
		u.MsgUser(toUser, "no results")
		return
	}
	for i := len(postlist.Order) - 1; i >= 0; i-- {
		nick := u.mc.GetUser(postlist.Posts[postlist.Order[i]].UserId).Username
		for _, post := range strings.Split(postlist.Posts[postlist.Order[i]].Message, "\n") {
			u.MsgUser(toUser, "<"+nick+"> "+post)
		}
	}

}

func api(u *User, toUser *User, args []string) {
	var r *http.Response
	var err error
	if strings.ToLower(args[0]) == "get" {
		r, err = u.mc.Client.DoApiGet(args[1], "", "")
	} else {
		r, err = u.mc.Client.DoApiPost(args[1], strings.Join(args[2:], " "))
	}
	if err != nil {
		u.MsgUser(toUser, fmt.Sprintf("API error", err.Error()))
		return
	}
	if r.Body != nil {
		b, _ := ioutil.ReadAll(r.Body)
		r.Body.Close()
		u.MsgUser(toUser, string(b))
	}
}

var cmds = map[string]Command{
	"logout":      Command{handler: logout, params: 0},
	"login":       Command{handler: login},
	"search":      Command{handler: search, login: true},
	"searchusers": Command{handler: searchUsers, login: true},
	"scrollback":  Command{handler: scrollback, login: true},
	"api":         Command{handler: api, login: true},
}

func (u *User) handleMMServiceBot(toUser *User, msg string) {
	commands := strings.Fields(msg)
	cmd, ok := cmds[strings.ToLower(commands[0])]
	if !ok {
		keys := make([]string, 0)
		for k := range cmds {
			keys = append(keys, k)
		}
		u.MsgUser(toUser, "possible commands: "+strings.Join(keys, ", "))
		u.MsgUser(toUser, "<command> help for more info")
		return
	}
	if cmd.login {
		if u.mc == nil {
			u.MsgUser(toUser, "You're not logged in. Use LOGIN first.")
			return
		}
	}
	cmd.handler(u, toUser, commands[1:])
}
