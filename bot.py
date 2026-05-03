import discord
from discord.ext import commands
import os
from flask import Flask, request
import threading
import requests
import json
import asyncio

app=Flask(__name__)

bot_Token = os.environ.get("BOT_TOKEN")
GUILD_ID = os.environ.get("GUILD_ID")


intents = discord.Intents.default()
intents.messages = True
intents.message_content = True
intents.guilds = True
intents.members = True

bot = commands.Bot(command_prefix='/', intents=intents)

@bot.event
async def on_ready():
    print("Bot Started")

async def create_channels(level):
    resp = requests.get("http://localhost:8080/bot/get", params={"ns": "level_channels", "key": level, "token": os.environ.get("BOT_TOKEN")})
    if resp.status_code == 200:
        return
    guild = bot.get_guild(int(GUILD_ID))
    levels_category = None
    hints_category = None
    for x in guild.categories:
        if x.name == "levels":
            levels_category = x
        if x.name == "hints":
            hints_category = x
    level_channel=await guild.create_text_channel(f"leads-{level}", category=levels_category)
    hint_channel=await guild.create_text_channel(f"hints-{level}", category=hints_category)
    requests.post("http://localhost:8080/bot/set", data={"ns": "level_channels", "key": level, "val": "{\"level\": %d, \"hint\": %d}" % (level_channel.id, hint_channel.id), "token": os.environ.get("BOT_TOKEN")})

@app.get("/create_level")
async def create_channel():
    asyncio.run_coroutine_threadsafe(create_channels(request.args["level"]), bot.loop)
    return {"succes":"created channels"}

async def send_message(level, name, email, content):
    resp = requests.get("http://localhost:8080/bot/get", params={"ns": "level_channels", "key": level, "token": os.environ.get("BOT_TOKEN")})
    if resp.status_code != 200:
        return
    entry = resp.json()
    if isinstance(entry, str):
        entry = json.loads(entry)
    channel_id = entry.get("level")
    channel=bot.get_channel(int(channel_id))
    message=await channel.send(f"`{name} {email} : {content}`\n")
    requests.post("http://localhost:8080/bot/set", data={"ns": "discord_messages", "key": str(message.id), "val": email, "token": os.environ.get("BOT_TOKEN")})

@app.get("/send_message")
async def send_message_api():
    asyncio.run_coroutine_threadsafe(send_message(request.args["level"], request.args["name"], request.args["email"], request.args["content"]), bot.loop)
    return {"success":"true"}

@bot.event
async def on_message(message:discord.Message):
    if message.author==bot:
        return
    await bot.process_commands(message)
    if message.channel.name=="announcements":
        requests.post("http://localhost:8080/bot/set", data={"ns": "announcements", "key": str(message.id), "val": message.content, "token": os.environ.get("BOT_TOKEN")})
        return
    if message.channel.category.name=="hints":
        level=message.channel.name.split("-")[1]
        requests.post("http://localhost:8080/bot/set", data={"ns": "hints/"+level, "key": str(message.id), "val": message.content, "token": os.environ.get("BOT_TOKEN")})
        return
    if message.reference!=None:
        id=message.reference.message_id
        resp = requests.get("http://localhost:8080/bot/get", params={"ns": "discord_messages", "key": str(id), "token": os.environ.get("BOT_TOKEN")})
        if resp.status_code == 200:
            email = resp.text.strip('"')
            requests.post("http://localhost:8080/bot/set", data={"ns": "messages/"+email, "key": str(message.id), "val": message.content, "token": os.environ.get("BOT_TOKEN")})

@bot.event
async def on_message_delete(message:discord.Message):
    if message.channel.name=="announcements":
        requests.get("http://localhost:8080/bot/delete", params={"ns": "announcements", "key": str(message.id), "token": os.environ.get("BOT_TOKEN")})
        return
    if message.channel.category.name=="hints":
        level=message.channel.name.split("-")[1]
        requests.get("http://localhost:8080/bot/delete", params={"ns": "hints/"+level, "key": str(message.id), "token": os.environ.get("BOT_TOKEN")})
    if message.reference!=None:
        id=message.reference.message_id
        resp = requests.get("http://localhost:8080/bot/get", params={"ns": "discord_messages", "key": str(id), "token": os.environ.get("BOT_TOKEN")})
        if resp.status_code == 200:
            email = resp.text.strip('"')
            requests.get("http://localhost:8080/bot/delete", params={"ns": "messages/"+email, "key": str(message.id), "token": os.environ.get("BOT_TOKEN")})

@bot.command()
async def info(ctx):
    await ctx.send("""
Commands:
```
/info : help page
/backlink : to set a backlink, example: /backlink abcd https://intra.sudocrypt.com/assets/logo.png
/logs : to get the logs of a player, example: /logs exun@dpsrkp.net
/leads : to toggle leads, example: /leads
/logs : to get the logs of a player, example: /logs exun@dpsrkp.net
/disqualify : to toggle disqualification of a player, example: /disqualify {email}
```
""")

@bot.command()
async def backlink(ctx, backlink, url):
    requests.post("http://localhost:8080/bot/set", data={"ns": "backlinks", "key": backlink, "val": url, "token": os.environ.get("BOT_TOKEN")})
    await ctx.send("backlink /"+backlink+" set to `"+url+"`")

@bot.command()
async def logs(ctx, email):
    resp = requests.get("http://localhost:8080/bot/get", params={"ns": "logs", "key": email, "token": os.environ.get("BOT_TOKEN")})
    if resp.status_code != 200:
        await ctx.send("no logs")
        return
    log = resp.text
    if len(log)>1800:
        log=log[len(log)-1800:]
    await ctx.send("```"+log+"```")

@bot.command()
async def leads(ctx):
    resp = requests.get("http://localhost:8080/bot/get", params={"ns": "status", "key": "leads", "token": os.environ.get("BOT_TOKEN")})
    current_Leads = False
    if resp.status_code == 200:
        current_Leads = resp.text.lower() in ("true", "1")
    requests.post("http://localhost:8080/bot/set", data={"ns": "status", "key": "leads", "val": str(not current_Leads).lower(), "token": os.environ.get("BOT_TOKEN")})
    message=""
    if current_Leads:
        message="off"
    else:
        message="on"
    await ctx.send("Leads have been turned "+message)

@bot.command()
async def disqualify(ctx, email):
    resp = requests.get("http://localhost:8080/bot/get", params={"ns": "disqualified", "key": email, "token": os.environ.get("BOT_TOKEN")})
    disqualified = False
    if resp.status_code == 200:
        disqualified = resp.text.lower() in ("true", "1")
    requests.post("http://localhost:8080/bot/set", data={"ns": "disqualified", "key": email, "val": str(not disqualified).lower(), "token": os.environ.get("BOT_TOKEN")})
    message=""
    if disqualified:
        message="allowed to play"
    else:
        message="disqualified"
    await ctx.send(email+" has been "+message)

@bot.event
async def on_message_edit(before, after):
    if before.channel.name=="announcements":
        resp = requests.get("http://localhost:8080/bot/get", params={"ns": "announcements", "key": str(before.id), "token": os.environ.get("BOT_TOKEN")})
        if resp.status_code == 200:
            requests.post("http://localhost:8080/bot/set", data={"ns": "announcements", "key": str(before.id), "val": after.content, "token": os.environ.get("BOT_TOKEN")})
        return
    if before.channel.category.name=="hints":
        level=before.channel.name.split("-")[1]
        resp = requests.get("http://localhost:8080/bot/get", params={"ns": "hints/"+level, "key": str(before.id), "token": os.environ.get("BOT_TOKEN")})
        if resp.status_code == 200:
            requests.post("http://localhost:8080/bot/set", data={"ns": "hints/"+level, "key": str(before.id), "val": after.content, "token": os.environ.get("BOT_TOKEN")})
    if before.reference!=None:
        id=before.reference.message_id
        resp = requests.get("http://localhost:8080/bot/get", params={"ns": "discord_messages", "key": str(id), "token": os.environ.get("BOT_TOKEN")})
        if resp.status_code == 200:
            email = resp.text.strip('"')
            requests.post("http://localhost:8080/bot/set", data={"ns": "messages/"+email, "key": str(before.id), "val": after.content, "token": os.environ.get("BOT_TOKEN")})

threading.Thread(target=bot.run, args=(bot_Token, ), daemon=True).start()
app.run(host="0.0.0.0", port=5555)
