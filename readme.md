I'm a user that tends to delete my comments after a couple of days. When the initial engagement of a post dies down. I hopped between two accounts one day and when I logged back into my main I noticed comments from 10 years ago showing that never showed before.

Having looked around for something that was already built that could help me out. Found a lot of javascript and python repos but they were all just meh to me.

Eventually I found Shreddit and I couldn't make it work after 5 minutes. Took the functionality I wanted from shreddit and converted it to Go.

This is not a feature rich or feature complete tool. You can honestly think of it as a simple script to delete all your reddit posts. Putting it out there for those that might want to use.

## Setup
You need to create an Oauth application for authentication. 

* https://www.reddit.com/prefs/apps Go there
* 'Create an app' and give it a name
* Select `script`
* callback url can be `http://localhost:8080`
* Click `create app`

A new page should load with the the secret and when you are looking at the application, just below the app name and 'personal use script' will be the `client id`

## Config

I got super lazy I know. I could have used a yaml or json config file, but environment variables are fine enough.

The `REDDIT_USER_AGENT` is required by reddit.

```shell
export REDDIT_USERNAME=snoo
export REDDIT_PASSWORD=foobar
export REDDIT_CLIENT_ID=123
export REDDIT_CLIENT_SECRET=123
export REDDIT_USER_AGENT=ShredditGoClient
export REDDIT_YEARS_BACK 11
export REDDIT_DRY_RUN false
```