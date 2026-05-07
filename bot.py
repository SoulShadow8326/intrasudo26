import os
import json
import asyncio
from typing import Optional
from dotenv import load_dotenv
from fastapi import FastAPI, Query, HTTPException
from contextlib import asynccontextmanager
import uvicorn
import aiohttp
import discord
from discord.ext import commands
from discord import app_commands
import logging
import sys
import io

load_dotenv()

BOT_TOKEN = os.environ.get("BOT_TOKEN")
BOT_API_TOKEN = os.environ.get("BOT_API_TOKEN") or BOT_TOKEN
GUILD_ID = os.environ.get("GUILD_ID")
BACKEND_BASE = os.environ.get("BACKEND_BASE", "http://localhost:8080")
HTTP_TIMEOUT = int(os.environ.get("HTTP_TIMEOUT", "5"))

intents = discord.Intents.default()
intents.messages = True
intents.message_content = True
intents.guilds = True
intents.members = True

bot = commands.Bot(command_prefix='/', intents=intents)
@asynccontextmanager
async def lifespan(app: FastAPI):
    asyncio.create_task(start_services())
    try:
        yield
    finally:
        await shutdown()

app = FastAPI(lifespan=lifespan)

session: Optional[aiohttp.ClientSession] = None

logger = logging.getLogger("bot")
logging.basicConfig(level=logging.INFO, stream=sys.stdout)


async def ensure_session():
    global session
    if session is None or session.closed:
        session = aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=HTTP_TIMEOUT))
    return session


async def backend_get(ns: str, key: str):
    s = await ensure_session()
    params = {"ns": ns, "key": key, "token": BOT_API_TOKEN}
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
    data = {"ns": ns, "key": key, "val": val, "token": BOT_API_TOKEN}
    try:
        async with s.post(f"{BACKEND_BASE}/bot/set", data=data) as resp:
            status = resp.status
            if status != 200:
                text = await resp.text()
                logger.warning("backend_post non-200 response: ns=%s key=%s status=%s text=%s", ns, key, status, text)
            return status
    except asyncio.CancelledError:
        raise
    except Exception:
        logger.exception("backend_post error: %s %s", ns, key)
        return 500


async def backend_delete(ns: str, key: str):
    s = await ensure_session()
    params = {"ns": ns, "key": key, "token": BOT_API_TOKEN}
    try:
        async with s.delete(f"{BACKEND_BASE}/bot/delete", params=params) as resp:
            status = resp.status
            if status != 200:
                text = await resp.text()
                logger.warning("backend_delete non-200 response: ns=%s key=%s status=%s text=%s", ns, key, status, text)
            return status
    except asyncio.CancelledError:
        raise
    except Exception:
        logger.exception("backend_delete error: %s %s", ns, key)
        return 500


async def backend_levels_count() -> int:
    s = await ensure_session()
    params = {"token": BOT_API_TOKEN}
    try:
        async with s.get(f"{BACKEND_BASE}/bot/levels/count", params=params) as resp:
            if resp.status != 200:
                return 0
            payload = await resp.json()
            return int(payload.get("count", 0))
    except asyncio.CancelledError:
        raise
    except Exception:
        logger.exception("backend_levels_count error")
        return 0


async def find_or_create_text_channel(guild: discord.Guild, name: str) -> Optional[discord.TextChannel]:
    for ch in guild.text_channels:
        if ch.name == name:
            return ch
    try:
        return await guild.create_text_channel(name)
    except Exception:
        logger.exception("failed to create text channel %s", name)
        return None


def get_text_channel_by_name(guild: discord.Guild, name: str) -> Optional[discord.TextChannel]:
    for ch in guild.text_channels:
        if ch.name == name:
            return ch
    return None


def parse_hint_format(content: str):
    raw = (content or "").strip()
    if not raw.lower().startswith("hint "):
        return None
    body = raw[5:].strip()
    if "|" not in body:
        return None
    level_id, text = body.split("|", 1)
    level_id = level_id.strip()
    text = text.strip()
    if not level_id or not text:
        return None
    return level_id, text


def parse_lead_format(content: str):
    raw = (content or "").strip()
    if not raw.lower().startswith("lead "):
        return None
    body = raw[5:].strip()
    if "|" not in body:
        return None
    email, text = body.split("|", 1)
    email = email.strip().lower()
    text = text.strip()
    if not email or not text:
        return None
    return email, text


async def ensure_format_help_message(channel: discord.TextChannel):
    marker = "[INTRASUDO FORMAT HELP]"
    try:
        async for msg in channel.history(limit=30):
            if msg.author == bot.user and marker in (msg.content or ""):
                return
    except Exception:
        logger.exception("failed to read channel history for %s", channel.name)
        return
    if channel.name == "hints":
        help_text = (
            f"{marker}\n"
            "Use this format:\n"
            "`hint <level_id> | <hint text>`\n"
            "Example:\n"
            "`hint cryptic-3 | Try reading the title backwards`"
        )
    else:
        help_text = (
            f"{marker}\n"
            "To send a lead to a player, **reply to their message** in this channel and type your lead.\n"
            "Example: reply to a player's message with `Focus on line 2 punctuation`"
        )
    try:
        await channel.send(help_text)
    except Exception:
        logger.exception("failed to send format help in %s", channel.name)


async def ensure_global_leads_hints_channels():
    if not GUILD_ID:
        return
    count = await backend_levels_count()
    if count <= 0:
        logger.info("skipping leads/hints channel setup because level count is %s", count)
        return
    guild = bot.get_guild(int(GUILD_ID))
    if guild is None:
        logger.error("Guild not found for global channel setup: %s", GUILD_ID)
        return
    leads_channel = await find_or_create_text_channel(guild, "leads")
    hints_channel = await find_or_create_text_channel(guild, "hints")
    if leads_channel:
        await ensure_format_help_message(leads_channel)
    if hints_channel:
        await ensure_format_help_message(hints_channel)


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
    guild = bot.get_guild(int(GUILD_ID))
    if guild is None:
        logger.error("Guild not found: %s", GUILD_ID)
        return
    channel = get_text_channel_by_name(guild, "leads")
    if channel is None:
        channel = await find_or_create_text_channel(guild, "leads")
    if channel is None:
        logger.error("could not get/create leads channel")
        return
    message = await channel.send(f"`[{level}] {name} {email} : {content}`")
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
    if message.channel is None:
        return
    if message.channel.name == "announcements":
        await backend_post("announcements", str(message.id), message.content)
        return
    if message.channel.name == "hints":
        parsed = parse_hint_format(message.content)
        if parsed:
            level_id, hint_text = parsed
            await backend_post(f"hints/{level_id}", str(message.id), hint_text)
        return
    if message.channel.name == "leads":
        if message.reference is not None:
            ref_id = message.reference.message_id
            status, text = await backend_get("discord_messages", str(ref_id))
            if status == 200:
                email = text.strip('"')
                await backend_post(f"messages/{email}", str(message.id), message.content)
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
    if message.channel.name == "hints":
        await backend_delete("hints/_", str(message.id))
        return
    if message.channel.name == "leads":
        await backend_delete("messages/_", str(message.id))
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


@bot.event
async def on_ready():
    logger.info("Bot Started")
    if GUILD_ID:
        try:
            guild = discord.Object(id=int(GUILD_ID))
            bot.tree.copy_global_to(guild=guild)
            await bot.tree.sync(guild=guild)
            logger.info("synced application commands to guild %s", GUILD_ID)
        except Exception:
            logger.exception("failed to sync commands to guild %s", GUILD_ID)
    try:
        await ensure_global_leads_hints_channels()
    except Exception:
        logger.exception("failed to ensure global leads/hints channels")


@app_commands.command(name="info")
async def info(interaction: discord.Interaction):
    embed = discord.Embed(title="Bot Commands", color=0x2F3136)
    embed.add_field(name="/info", value="Show this help message", inline=False)
    embed.add_field(name="/backlink <backlink> <url>", value="Set a backlink to a URL", inline=False)
    embed.add_field(name="/logs <email>", value="Get logs for a player", inline=False)
    embed.add_field(name="/leads", value="Toggle leads on/off", inline=False)
    embed.add_field(name="/disqualify <email>", value="Toggle disqualification for a player", inline=False)
    await interaction.response.send_message(embed=embed)


@app_commands.command(name="backlink")
@app_commands.describe(backlink="backlink", url="url")
async def backlink(interaction: discord.Interaction, backlink: str, url: str):
    await backend_post("backlinks", backlink, url)
    embed = discord.Embed(title="Backlink Set", color=0x2F3136)
    embed.add_field(name="Backlink", value=f"/{backlink}", inline=True)
    embed.add_field(name="URL", value=url, inline=True)
    await interaction.response.send_message(embed=embed)


@app_commands.command(name="logs")
@app_commands.describe(email="player email")
async def logs(interaction: discord.Interaction, email: str):
    status, text = await backend_get("logs", email)
    if status != 200:
        embed = discord.Embed(title="Logs", description="no logs found for provided email", color=0xFF0000)
        await interaction.response.send_message(embed=embed)
        return
    log = text
    if len(log) > 1900:
        bio = io.BytesIO(log.encode())
        bio.seek(0)
        await interaction.response.send_message(file=discord.File(fp=bio, filename=f"logs_{email}.txt"))
        return
    embed = discord.Embed(title="Logs", color=0x2F3136)
    embed.description = f"```{log}```"
    await interaction.response.send_message(embed=embed)


@app_commands.command(name="leads")
@app_commands.describe(level="level id")
async def leads(interaction: discord.Interaction, level: str):
    status, text = await backend_get("status", level)
    current_Leads = False
    if status == 200:
        current_Leads = text.lower() in ("true", "1")
    await backend_post("status", level, str(not current_Leads).lower())
    message = "on" if not current_Leads else "off"
    embed = discord.Embed(title="Leads Toggled", color=0x2F3136)
    embed.add_field(name="Level", value=level, inline=True)
    embed.add_field(name="Status", value=message, inline=True)
    await interaction.response.send_message(embed=embed)


@app_commands.command(name="disqualify")
@app_commands.describe(email="player email")
async def disqualify(interaction: discord.Interaction, email: str):
    status, text = await backend_get("disqualified", email)
    disqualified = False
    if status == 200:
        disqualified = text.lower() in ("true", "1")
    await backend_post("disqualified", email, str(not disqualified).lower())
    message = "allowed to play" if disqualified else "disqualified"
    embed = discord.Embed(title="Disqualification Toggled", color=0x2F3136)
    embed.add_field(name="Email", value=email, inline=True)
    embed.add_field(name="Status", value=message, inline=True)
    await interaction.response.send_message(embed=embed)


bot.tree.add_command(info)
bot.tree.add_command(backlink)
bot.tree.add_command(logs)
bot.tree.add_command(leads)
bot.tree.add_command(disqualify)


@bot.event
async def on_message_edit(before, after):
    if before.channel is None:
        return
    if before.channel.name == "announcements":
        status, _ = await backend_get("announcements", str(before.id))
        if status == 200:
            await backend_post("announcements", str(before.id), after.content)
        return
    if before.channel.name == "hints":
        parsed = parse_hint_format(after.content)
        if parsed:
            level_id, hint_text = parsed
            await backend_post(f"hints/{level_id}", str(before.id), hint_text)
        return
        if before.channel.name == "leads":
            if before.reference is not None:
                ref_id = before.reference.message_id
                status, text = await backend_get("discord_messages", str(ref_id))
                if status == 200:
                    email = text.strip('"')
                    await backend_post(f"messages/{email}", str(before.id), after.content)
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


async def shutdown():
    global session
    logger.info("shutting down: closing http session and logging out bot")
    if session is not None and not session.closed:
        await session.close()
    try:
        await bot.close()
    except Exception:
        logger.exception("error while closing bot")



if __name__ == "__main__":
    uvicorn.run("bot:app", host="0.0.0.0", port=5555, log_level="info")
