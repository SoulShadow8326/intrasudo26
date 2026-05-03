import os
import json
import asyncio
from typing import Optional
from dotenv import load_dotenv
from fastapi import FastAPI, Query, HTTPException
import uvicorn
import aiohttp
import discord
from discord.ext import commands
import logging
import sys

load_dotenv()

BOT_TOKEN = os.environ.get("BOT_TOKEN")
GUILD_ID = os.environ.get("GUILD_ID")
BACKEND_BASE = os.environ.get("BACKEND_BASE", "http://localhost:8080")
HTTP_TIMEOUT = int(os.environ.get("HTTP_TIMEOUT", "5"))

intents = discord.Intents.default()
intents.messages = True
intents.message_content = True
intents.guilds = True
intents.members = True

bot = commands.Bot(command_prefix='/', intents=intents)
app = FastAPI()

session: Optional[aiohttp.ClientSession] = None

logger = logging.getLogger("bot")
logging.basicConfig(level=logging.INFO, stream=sys.stdout)


@bot.event
async def on_ready():
    logger.info("Bot Started")


async def ensure_session():
    global session
    if session is None or session.closed:
        session = aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=HTTP_TIMEOUT))
    return session


async def backend_get(ns: str, key: str):
    s = await ensure_session()
    params = {"ns": ns, "key": key, "token": BOT_TOKEN}
    try:
        async with s.get(f"{BACKEND_BASE}/bot/get", params=params) as resp:
            status = resp.status
            text = await resp.text()
            return status, text
    except asyncio.CancelledError:
        raise
    except Exception:
        logger.exception("backend_get error: %s %s", ns, key)
        return 500, ""


async def backend_post(ns: str, key: str, val: str):
    s = await ensure_session()
    data = {"ns": ns, "key": key, "val": val, "token": BOT_TOKEN}
    try:
        async with s.post(f"{BACKEND_BASE}/bot/set", data=data) as resp:
            return resp.status
    except asyncio.CancelledError:
        raise
    except Exception:
        logger.exception("backend_post error: %s %s", ns, key)
        return 500


async def backend_delete(ns: str, key: str):
    s = await ensure_session()
    params = {"ns": ns, "key": key, "token": BOT_TOKEN}
    try:
        async with s.get(f"{BACKEND_BASE}/bot/delete", params=params) as resp:
            return resp.status
    except asyncio.CancelledError:
        raise
    except Exception:
        logger.exception("backend_delete error: %s %s", ns, key)
        return 500


async def find_or_create_category(guild: discord.Guild, name: str) -> Optional[discord.CategoryChannel]:
    for c in guild.categories:
        if c.name == name:
            return c
    try:
        return await guild.create_category(name)
    except Exception:
        return None


async def create_channels(level: str):
    status, _ = await backend_get("level_channels", level)
    if status == 200:
        return
    guild = bot.get_guild(int(GUILD_ID))
    if guild is None:
        logger.error("Guild not found: %s", GUILD_ID)
        return
    levels_category = await find_or_create_category(guild, "levels")
    hints_category = await find_or_create_category(guild, "hints")
    level_channel = await guild.create_text_channel(f"leads-{level}", category=levels_category)
    hint_channel = await guild.create_text_channel(f"hints-{level}", category=hints_category)
    val = json.dumps({"level": level_channel.id, "hint": hint_channel.id})
    try:
        await backend_post("level_channels", level, val)
    except Exception:
        logger.exception("failed to post level channels for %s", level)


@app.get("/create_level")
async def create_channel_endpoint(level: str = Query(...)):
    try:
        asyncio.create_task(create_channels(level))
    except Exception:
        raise HTTPException(status_code=500, detail="failed to schedule channel creation")
    return {"success": "created channels"}


async def send_message(level: str, name: str, email: str, content: str):
    status, text = await backend_get("level_channels", level)
    if status != 200:
        return
    try:
        entry = json.loads(text) if isinstance(text, str) else text
    except Exception:
        logger.exception("failed to parse level_channels response for %s", level)
        return
    channel_id = entry.get("level")
    if channel_id is None:
        return
    channel = bot.get_channel(int(channel_id))
    if channel is None:
        logger.error("channel not found: %s", channel_id)
        return
    message = await channel.send(f"`{name} {email} : {content}`\n")
    try:
        await backend_post("discord_messages", str(message.id), email)
    except Exception:
        logger.exception("failed to record discord message %s", message.id)


@app.get("/send_message")
async def send_message_api(level: str = Query(...), name: str = Query(...), email: str = Query(...), content: str = Query(...)):
    try:
        asyncio.create_task(send_message(level, name, email, content))
    except Exception:
        raise HTTPException(status_code=500, detail="failed to schedule send_message")
    return {"success": "true"}


@bot.event
async def on_message(message: discord.Message):
    if message.author == bot.user:
        return
    await bot.process_commands(message)
    if message.channel is None:
        return
    if message.channel.name == "announcements":
        await backend_post("announcements", str(message.id), message.content)
        return
    if message.channel.category and message.channel.category.name == "hints":
        parts = message.channel.name.split("-")
        if len(parts) > 1:
            level = parts[1]
            await backend_post(f"hints/{level}", str(message.id), message.content)
        return
    if message.reference is not None:
        id = message.reference.message_id
        status, text = await backend_get("discord_messages", str(id))
        if status == 200:
            email = text.strip('"')
            await backend_post(f"messages/{email}", str(message.id), message.content)


@bot.event
async def on_message_delete(message: discord.Message):
    if message.channel is None:
        return
    if message.channel.name == "announcements":
        await backend_delete("announcements", str(message.id))
        return
    if message.channel.category and message.channel.category.name == "hints":
        parts = message.channel.name.split("-")
        if len(parts) > 1:
            level = parts[1]
            await backend_delete(f"hints/{level}", str(message.id))
    if message.reference is not None:
        id = message.reference.message_id
        status, text = await backend_get("discord_messages", str(id))
        if status == 200:
            email = text.strip('"')
            await backend_delete(f"messages/{email}", str(message.id))


@bot.command()
async def info(ctx):
    await ctx.send("Commands:\n``/info /backlink /logs /leads /disqualify``")


@bot.command()
async def backlink(ctx, backlink: str, url: str):
    await backend_post("backlinks", backlink, url)
    await ctx.send(f"backlink /{backlink} set to `{url}`")


@bot.command()
async def logs(ctx, email: str):
    status, text = await backend_get("logs", email)
    if status != 200:
        await ctx.send("no logs")
        return
    log = text
    if len(log) > 1800:
        log = log[len(log) - 1800:]
    await ctx.send(f"```{log}```")


@bot.command()
async def leads(ctx):
    status, text = await backend_get("status", "leads")
    current_Leads = False
    if status == 200:
        current_Leads = text.lower() in ("true", "1")
    await backend_post("status", "leads", str(not current_Leads).lower())
    message = "on" if not current_Leads else "off"
    await ctx.send("Leads have been turned " + message)


@bot.command()
async def disqualify(ctx, email: str):
    status, text = await backend_get("disqualified", email)
    disqualified = False
    if status == 200:
        disqualified = text.lower() in ("true", "1")
    await backend_post("disqualified", email, str(not disqualified).lower())
    message = "allowed to play" if disqualified else "disqualified"
    await ctx.send(email + " has been " + message)


@bot.event
async def on_message_edit(before, after):
    if before.channel is None:
        return
    if before.channel.name == "announcements":
        status, _ = await backend_get("announcements", str(before.id))
        if status == 200:
            await backend_post("announcements", str(before.id), after.content)
        return
    if before.channel.category and before.channel.category.name == "hints":
        parts = before.channel.name.split("-")
        if len(parts) > 1:
            level = parts[1]
            status, _ = await backend_get(f"hints/{level}", str(before.id))
            if status == 200:
                await backend_post(f"hints/{level}", str(before.id), after.content)
        return
    if before.reference is not None:
        id = before.reference.message_id
        status, text = await backend_get("discord_messages", str(id))
        if status == 200:
            email = text.strip('"')
            await backend_post(f"messages/{email}", str(before.id), after.content)


async def start_services():
    await ensure_session()
    if not BOT_TOKEN or not GUILD_ID:
        logger.error("BOT_TOKEN or GUILD_ID not set")
        return
    try:
        task = asyncio.create_task(bot.start(BOT_TOKEN))
        def _on_done(t):
            try:
                t.result()
            except asyncio.CancelledError:
                logger.info("bot.start cancelled")
            except Exception:
                logger.exception("bot task failed")
        task.add_done_callback(_on_done)
        logger.info("bot start scheduled")
    except Exception:
        logger.exception("failed to start bot")


@app.on_event("shutdown")
async def shutdown():
    global session
    logger.info("shutting down: closing http session and logging out bot")
    if session is not None and not session.closed:
        await session.close()
    try:
        await bot.close()
    except Exception:
        logger.exception("error while closing bot")


@app.on_event("startup")
async def startup_event():
    asyncio.create_task(start_services())


if __name__ == "__main__":
    uvicorn.run("bot:app", host="0.0.0.0", port=5555, log_level="info")
