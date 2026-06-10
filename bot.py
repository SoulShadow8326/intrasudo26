import os
import json
import asyncio
from datetime import datetime, timezone
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
BACKEND_BASE = os.environ.get("BACKEND_BASE", "http://127.0.0.1:8080")
HTTP_TIMEOUT = int(os.environ.get("HTTP_TIMEOUT", "15"))
BOT_HOST = os.environ.get("BOT_HOST", "127.0.0.1")
BOT_PORT = int(os.environ.get("BOT_PORT", "5555"))
HEALTH_CHANNEL = os.environ.get("HEALTH_CHANNEL")
_health_check_user_id = os.environ.get("HEALTH_CHECK_USER_ID")
HEALTH_CHECK_USER_ID = int(_health_check_user_id) if _health_check_user_id else None
HEALTH_CHECK_INTERVAL = 300

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
health_check_failed = False

logger = logging.getLogger("bot")
logging.basicConfig(level=logging.INFO, stream=sys.stdout)


async def ensure_session():
    global session
    if session is None or session.closed:
        session = aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=HTTP_TIMEOUT))
    return session


async def backend_get(ns: str, key: str):
    s = await ensure_session()
    params = {"ns": ns, "key": key}
    headers = {"X-BOT-TOKEN": BOT_API_TOKEN}
    try:
        async with s.get(f"{BACKEND_BASE}/bot/get", params=params, headers=headers) as resp:
            status = resp.status
            text = await resp.text()
            return status, text
    except asyncio.CancelledError:
        raise
    except Exception:
        logger.exception("backend_get error: %s %s", ns, key)
        return 500, ""


async def backend_post(ns: str, key: str, val: str, reply_to: str = ""):
    s = await ensure_session()
    data = {"ns": ns, "key": key, "val": val}
    if reply_to:
        data["reply_to"] = reply_to
    headers = {"X-BOT-TOKEN": BOT_API_TOKEN}
    try:
        async with s.post(f"{BACKEND_BASE}/bot/set", data=data, headers=headers) as resp:
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


async def backend_get_thread(email: str, limit: int = 50):
    s = await ensure_session()
    headers = {"X-BOT-TOKEN": BOT_API_TOKEN}
    try:
        async with s.get(f"{BACKEND_BASE}/bot/thread", params={"email": email, "limit": limit}, headers=headers) as resp:
            status = resp.status
            if status == 404:
                return status, None
            if status != 200:
                text = await resp.text()
                logger.warning("backend_get_thread non-200: email=%s status=%s text=%s", email, status, text)
                return status, None
            payload = await resp.json()
            return status, payload
    except asyncio.CancelledError:
        raise
    except Exception:
        logger.exception("backend_get_thread error: %s", email)
        return 500, None


async def backend_reset(email: str, level: str):
    s = await ensure_session()
    data = {"email": email, "level": level}
    headers = {"X-BOT-TOKEN": BOT_API_TOKEN}
    try:
        async with s.post(f"{BACKEND_BASE}/bot/reset", data=data, headers=headers) as resp:
            status = resp.status
            if status not in (200, 404):
                text = await resp.text()
                logger.warning("backend_reset non-200: email=%s level=%s status=%s text=%s", email, level, status, text)
            return status
    except asyncio.CancelledError:
        raise
    except Exception:
        logger.exception("backend_reset error: %s %s", email, level)
        return 500


async def backend_delete(ns: str, key: str):
    s = await ensure_session()
    params = {"ns": ns, "key": key}
    headers = {"X-BOT-TOKEN": BOT_API_TOKEN}
    try:
        async with s.delete(f"{BACKEND_BASE}/bot/delete", params=params, headers=headers) as resp:
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
    headers = {"X-BOT-TOKEN": BOT_API_TOKEN}
    try:
        async with s.get(f"{BACKEND_BASE}/bot/levels/count", headers=headers) as resp:
            if resp.status != 200:
                return 0
            payload = await resp.json()
            return int(payload.get("count", 0))
    except asyncio.CancelledError:
        raise
    except Exception:
        logger.exception("backend_levels_count error")
        return 0


async def check_health():
    global health_check_failed
    s = await ensure_session()
    headers = {"X-BOT-TOKEN": BOT_API_TOKEN}
    try:
        async with s.get(f"{BACKEND_BASE}/bot/audit", headers=headers) as resp:
            if resp.status != 200:
                health_check_failed = True
                await report_health(f"Unhealthy status: {resp.status}", unhealthy=True)
                return False
            payload = await resp.json()
            health_check_failed = False
            await report_health("Healthy", payload=payload, unhealthy=False)
            return True
    except asyncio.CancelledError:
        raise
    except (asyncio.TimeoutError, aiohttp.ClientError) as e:
        health_check_failed = True
        await report_health(f"Connection failed or timed out: {e}", unhealthy=True)
        return False
    except Exception:
        health_check_failed = True
        await report_health("Unexpected error during health check", unhealthy=True)
        return False


async def report_health(reason: str, payload: dict = None, unhealthy: bool = False):
    if not HEALTH_CHANNEL or not GUILD_ID:
        return
    try:
        guild = bot.get_guild(int(GUILD_ID))
        if not guild:
            return
        channel = guild.get_channel(int(HEALTH_CHANNEL))
        if not channel:
            return

        mention = f"<@{HEALTH_CHECK_USER_ID}> " if unhealthy and HEALTH_CHECK_USER_ID else ""
        color = 0xFF0000 if unhealthy else 0x00FF00
        title = "Server Health Alert" if unhealthy else "Server Health Report"

        embed = discord.Embed(title=title, description=reason, color=color)
        if payload and "metrics" in payload:
            m = payload.get("metrics", {})
            total_reqs = m.get("total_requests", 0)
            embed.add_field(name="Total Requests", value=str(total_reqs), inline=True)

            if total_reqs > 0:
                total_duration = m.get("total_duration_ms", 0)
                avg_latency = total_duration / total_reqs
                embed.add_field(name="Avg Latency", value=f"{avg_latency:.2f}ms", inline=True)

            methods = m.get("requests_by_method", {})
            if methods:
                method_str = "\n".join([f"{k}: {v}" for k, v in methods.items()])
                embed.add_field(name="Requests by Method", value=f"```\n{method_str}\n```", inline=False)

            statuses = m.get("status_counts", {})
            if statuses:
                status_str = "\n".join([f"{k}: {v}" for k, v in statuses.items()])
                embed.add_field(name="Status Codes", value=f"```\n{status_str}\n```", inline=False)

            paths = m.get("requests_by_path", {})
            if paths:
                top_paths = sorted(paths.items(), key=lambda x: x[1], reverse=True)[:5]
                path_str = "\n".join([f"{k}: {v}" for k, v in top_paths])
                embed.add_field(name="Top Paths", value=f"```\n{path_str}\n```", inline=False)

        await channel.send(content=mention if mention else None, embed=embed)
    except Exception:
        logger.exception("failed to send health report")


health_check_task = None

async def health_check_loop():
    while not bot.is_closed():
        try:
            await check_health()
        except Exception:
            logger.exception("health check loop error")
        await asyncio.sleep(HEALTH_CHECK_INTERVAL)


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


def parse_discord_msg_data(text: str) -> tuple:
    raw = (text or "").strip()
    try:
        outer = json.loads(raw)
    except (json.JSONDecodeError, ValueError):
        return raw.strip('"').lower(), ""
    if isinstance(outer, dict):
        return outer.get("email", "").strip().lower(), outer.get("content", "").strip()
    if isinstance(outer, str):
        try:
            inner = json.loads(outer)
            if isinstance(inner, dict):
                return inner.get("email", "").strip().lower(), inner.get("content", "").strip()
        except (json.JSONDecodeError, ValueError):
            pass
        return outer.strip().lower(), ""
    return str(outer).strip().lower(), ""


async def ensure_format_help_message(channel: discord.TextChannel):
    marker = "[INTRASUDO FORMAT HELP]"
    try:
        async for msg in channel.history(limit=30):
            if msg.author == bot.user and marker in (msg.content or ""):
                return
    except Exception:
        logger.exception("failed to read channel history for %s", channel.name)
        return
    name = (channel.name or "").lower()
    if name.startswith("hints") or (channel.category and getattr(channel.category, "name", "") == "hints"):
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
    for ch in guild.text_channels:
        try:
            name = (ch.name or "").lower()
            if name.startswith("leads-"):
                await ensure_format_help_message(ch)
            if name.startswith("hints-"):
                await ensure_format_help_message(ch)
        except Exception:
            logger.exception("failed ensuring format help for channel %s", getattr(ch, "name", "<unknown>"))


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
    existing_level = get_text_channel_by_name(guild, f"leads-{level}")
    existing_hint = get_text_channel_by_name(guild, f"hints-{level}")
    levels_category = await find_or_create_category(guild, "levels")
    hints_category = await find_or_create_category(guild, "hints")
    level_channel = existing_level or await guild.create_text_channel(f"leads-{level}", category=levels_category)
    hint_channel = existing_hint or await guild.create_text_channel(f"hints-{level}", category=hints_category)
    val = json.dumps({"level": level_channel.id, "hint": hint_channel.id})
    try:
        await backend_post("level_channels", level, val)
    except Exception:
        logger.exception("failed to post level channels for %s", level)
    try:
        if level_channel:
            await ensure_format_help_message(level_channel)
        if hint_channel:
            await ensure_format_help_message(hint_channel)
    except Exception:
        logger.exception("failed to ensure format help for level channels %s", level)

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

    status, uselessfellow = await backend_get("level_channels", level)
    if status != 200:
        await create_channels(level)

    channel = None
    status, text = await backend_get("level_channels", level)
    if status == 200:
        try:
            ids = json.loads(text)
            if isinstance(ids, str):
                ids = json.loads(ids)
            channel_id = ids.get("level")
            if channel_id:
                channel = guild.get_channel(int(channel_id))
        except Exception:
            logger.exception("failed to parse level_channels for %s", level)

    if channel is None:
        logger.error("level channel not found for %s, no fallback configured", level)
        return
    embed = discord.Embed(description=content, color=0x2977f5)
    embed.set_author(name=f"{name} ({email})")
    embed.set_footer(text=level)
    message = await channel.send(embed=embed)
    try:
        await backend_post("discord_messages", str(message.id), json.dumps({"email": email, "content": content}))
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
    if not hasattr(message.channel, 'name'):
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
                email, reply_to = parse_discord_msg_data(text)
                if email:
                    await backend_post(f"messages/{email}", str(message.id), message.content, reply_to=reply_to)
        return
    if message.channel.category and message.channel.category.name == "hints":
        parts = message.channel.name.split("-")
        if len(parts) > 1:
            level = "-".join(parts[1:])
            await backend_post(f"hints/{level}", str(message.id), message.content)
        return
    if message.reference is not None:
        id = message.reference.message_id
        status, text = await backend_get("discord_messages", str(id))
        if status == 200:
            email, reply_to = parse_discord_msg_data(text)
            if email:
                await backend_post(f"messages/{email}", str(message.id), message.content, reply_to=reply_to)


@bot.event
async def on_message_delete(message: discord.Message):
    if message.channel is None:
        return
    if not hasattr(message.channel, 'name'):
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
            level = "-".join(parts[1:])
            await backend_delete(f"hints/{level}", str(message.id))
    if message.reference is not None:
        id = message.reference.message_id
        status, text = await backend_get("discord_messages", str(id))
        if status == 200:
            email, _ = parse_discord_msg_data(text)
            if email:
                await backend_delete(f"messages/{email}", str(message.id))


@bot.event
async def on_ready():
    global health_check_task
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
    if health_check_task is None:
        health_check_task = asyncio.create_task(health_check_loop())
        logger.info("health check loop started")


@app_commands.command(name="info")
async def info(interaction: discord.Interaction):
    embed = discord.Embed(title="Bot Commands", color=0x2F3136)
    embed.add_field(name="/info", value="Show this help message", inline=False)
    embed.add_field(name="/backlink <backlink> <url>", value="Set a backlink to a URL", inline=False)
    embed.add_field(name="/logs <email>", value="Get logs for a player", inline=False)
    embed.add_field(name="/leads <level>", value="Toggle leads on/off for a level", inline=False)
    embed.add_field(name="/disqualify <email>", value="Toggle disqualification for a player", inline=False)
    embed.add_field(name="/thread <email>", value="Show last 50 messages for a player (ephemeral)", inline=False)
    embed.add_field(name="/reset <email> <level>", value="Reset a player to a specific level (e.g. cryptic-0)", inline=False)
    await interaction.response.send_message(embed=embed)


@app_commands.command(name="backlink")
@app_commands.describe(backlink="backlink", url="url")
async def backlink(interaction: discord.Interaction, backlink: str, url: str):
    clean_backlink = (backlink or "").strip()
    clean_url = (url or "").strip()
    if clean_backlink.startswith("/"):
        clean_backlink = clean_backlink.lstrip("/")
    if not clean_backlink or not clean_url:
        embed = discord.Embed(title="Backlink Set", description="backlink and url are required", color=0xFF0000)
        await interaction.response.send_message(embed=embed)
        return
    status = await backend_post("backlinks", clean_backlink, clean_url)
    if status != 200:
        embed = discord.Embed(title="Backlink Set", description="failed to set backlink", color=0xFF0000)
        await interaction.response.send_message(embed=embed)
        return
    embed = discord.Embed(title="Backlink Set", color=0x2F3136)
    embed.add_field(name="Backlink", value=f"/{clean_backlink}", inline=True)
    embed.add_field(name="URL", value=clean_url, inline=True)
    await interaction.response.send_message(embed=embed)


@app_commands.command(name="logs")
@app_commands.describe(email="player email")
async def logs(interaction: discord.Interaction, email: str):
    clean_email = (email or "").strip().lower()
    if not clean_email:
        embed = discord.Embed(title="Logs", description="email is required", color=0xFF0000)
        await interaction.response.send_message(embed=embed)
        return
    status, text = await backend_get("logs", clean_email)
    if status != 200 or not (text or "").strip():
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


@app_commands.command(name="thread")
@app_commands.describe(email="player email to view message thread for")
async def thread(interaction: discord.Interaction, email: str):
    clean_email = (email or "").strip().lower()
    if not clean_email:
        await interaction.response.send_message("Email is required.", ephemeral=True)
        return
    await interaction.response.defer(ephemeral=True)
    status, payload = await backend_get_thread(clean_email, limit=50)
    if status == 404:
        await interaction.followup.send(f"No account found for `{clean_email}`.", ephemeral=True)
        return
    if status != 200 or payload is None:
        await interaction.followup.send("Failed to fetch thread.", ephemeral=True)
        return
    messages = payload.get("messages") or []
    if not messages:
        await interaction.followup.send(f"No messages found for `{clean_email}`.", ephemeral=True)
        return
    lines = []
    for msg in messages:
        author = msg.get("author") or "?"
        content = msg.get("content") or ""
        reply_to = msg.get("reply_to") or ""
        t = msg.get("time") or 0
        dt = datetime.fromtimestamp(t, tz=timezone.utc).strftime("%d/%m %H:%M") if t else "?"
        prefix = "User" if msg.get("kind") != "hint" else "dot"
        if reply_to:
            lines.append(f"  ↩ _{reply_to[:60]}{'...' if len(reply_to) > 60 else ''}_")
        lines.append(f"{prefix} **{author}** [{dt}]: {content}")
    full = "\n".join(lines)
    chunks = [full[i:i+1900] for i in range(0, len(full), 1900)]
    header = f"**Thread for `{clean_email}`** — last {len(messages)} messages\n\n"
    await interaction.followup.send(header + chunks[0], ephemeral=True)
    for chunk in chunks[1:]:
        await interaction.followup.send(chunk, ephemeral=True)


@app_commands.command(name="reset")
@app_commands.describe(email="player email", level="level id to reset to (e.g. cryptic-0)")
async def reset(interaction: discord.Interaction, email: str, level: str):
    clean_email = (email or "").strip().lower()
    clean_level = (level or "").strip()
    if not clean_email or not clean_level:
        embed = discord.Embed(title="Reset Player", description="email and level are required", color=0xFF0000)
        await interaction.response.send_message(embed=embed, ephemeral=True)
        return
    status = await backend_reset(clean_email, clean_level)
    if status == 404:
        embed = discord.Embed(title="Reset Player", description=f"No account found for `{clean_email}`", color=0xFF0000)
        await interaction.response.send_message(embed=embed, ephemeral=True)
        return
    if status != 200:
        embed = discord.Embed(title="Reset Player", description="Failed to reset player", color=0xFF0000)
        await interaction.response.send_message(embed=embed, ephemeral=True)
        return
    embed = discord.Embed(title="Player Reset", color=0x2F3136)
    embed.add_field(name="Email", value=clean_email, inline=True)
    embed.add_field(name="Level", value=clean_level, inline=True)
    await interaction.response.send_message(embed=embed)


@app_commands.command(name="status")
async def status(interaction: discord.Interaction):
    s = await ensure_session()
    headers = {"X-BOT-TOKEN": BOT_API_TOKEN}
    try:
        async with s.get(f"{BACKEND_BASE}/bot/audit", headers=headers) as resp:
            if resp.status != 200:
                await interaction.response.send_message(f"failed to get audit: {resp.status}")
                return
            payload = await resp.json()
    except Exception:
        logger.exception("failed to fetch audit")
        await interaction.response.send_message("failed to fetch audit")
        return

    levels = payload.get("levels", [])
    statuses = payload.get("statuses", {})
    total_accounts = payload.get("total_accounts", 0)
    accounts_by_level = payload.get("accounts_by_level", {})
    hints_by_level = payload.get("hints_by_level", {})

    lines = []
    lines.append(f"Total levels: {len(levels)}")
    lines.append(f"Total accounts: {total_accounts}")
    lines.append("")
    lines.append("Level | Leads | Accounts | Hints")
    lines.append("----- | ----- | -------- | -----")
    for lv in levels:
        lid = lv.get("id") if isinstance(lv, dict) else str(lv)
        leads = False
        if lid in statuses:
            st = statuses.get(lid)
            if isinstance(st, dict):
                leads = st.get("leads", False)
        accounts = accounts_by_level.get(lid, 0)
        hints = hints_by_level.get(lid, 0)
        lines.append(f"{lid} | {str(leads)} | {accounts} | {hints}")

    msg = "\n".join(lines)
    try:
        desc = "```\n" + msg + "\n```"
        embed = discord.Embed(title="Audit Status", description=desc, color=0x2F3136)
        await interaction.response.send_message(embed=embed)
    except Exception:
        await interaction.response.send_message(msg)


bot.tree.add_command(info)
bot.tree.add_command(backlink)
bot.tree.add_command(logs)
bot.tree.add_command(leads)
bot.tree.add_command(disqualify)
bot.tree.add_command(thread)
bot.tree.add_command(reset)
bot.tree.add_command(status)


@bot.event
async def on_message_edit(before, after):
    if before.channel is None:
        return
    if not hasattr(before.channel, 'name'):
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
            level = "-".join(parts[1:])
            status, _ = await backend_get(f"hints/{level}", str(before.id))
            if status == 200:
                await backend_post(f"hints/{level}", str(before.id), after.content)
        return
    if before.reference is not None:
        id = before.reference.message_id
        status, text = await backend_get("discord_messages", str(id))
        if status == 200:
            email, reply_to = parse_discord_msg_data(text)
            if email:
                await backend_post(f"messages/{email}", str(before.id), after.content, reply_to=reply_to)


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
    uvicorn.run("bot:app", host=BOT_HOST, port=BOT_PORT, log_level="info")
