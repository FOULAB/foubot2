package configuration

const BotChannel = "#foulab"
const BotNick = "foubot"
const BotPswd = "HAHAHAHAHA, nope."
// Controls auto-voicing and the !vox command
const BotAutoVoice = false
const ServerTLS = "irc.libera.chat:6697"
// Set topic through chanserv instead of directly, avoids
// the need for the +o mode but requires chanserv flag +t
const TopicUseChanserv = true
const StatusEndPoint = "https://foulab.org/YTDMOWI3N2MXNMEZYWE4MGRHYTRLMZC4NJU5MJI2ZJYYODMYNME5NSAGLQO/"
const Blinker = "http://blinker.lab/"

// If set, update Mattermost channel header with the status of the lab
// (open or closed).
const MattermostServer = ""
const MattermostChannelId = ""

// https://developers.mattermost.com/integrate/reference/personal-access-token/
const MattermostToken = ""
