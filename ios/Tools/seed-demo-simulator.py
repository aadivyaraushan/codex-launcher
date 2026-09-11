#!/usr/bin/env python3
"""Seed a booted-once simulator with believable Calendar and Reminders data.

Why this exists
---------------
A fresh simulator ships with Apple's sample *contacts* (Kate Bell, John
Appleseed and four others, with 25 phone numbers between them) and with
birthdays and US holidays derived from them - but with no ordinary events and
no reminders at all. That is enough to demo `contacts.search`, and not enough
to demo anything else: "what's on my plate today?" correctly answers "nothing".

EventKit has no command-line seeding path and `idb` is not a dependency we
want, so this writes the rows straight into the CoreSimulator Calendar store.
`Calendar.supported_entity_types` uses 4 for events and 8 for reminders, which
are 1<<2 and 1<<3 - so `CalendarItem.entity_type` is 2 for an event and 3 for
a reminder. Calendar 2 is the default event calendar, calendar 3 the default
task calendar.

Usage
-----
    # shut the simulator down first, then:
    python3 ios/Tools/seed-demo-simulator.py <device-udid>

Back the store up before the first run; this writes to real simulator state:

    cp ~/Library/Developer/CoreSimulator/Devices/<udid>/data/Library/\
Calendar/Calendar.sqlitedb /somewhere/Calendar.sqlitedb.bak

Re-running is a no-op: it refuses to seed twice.
"""

import datetime
import os
import sqlite3
import sys
import uuid

APPLE_EPOCH = 978307200  # seconds between 1970-01-01 and 2001-01-01
TZ = "America/Los_Angeles"

EVENT_CALENDAR = 2
REMINDER_CALENDAR = 3
ENTITY_EVENT = 2
ENTITY_REMINDER = 3


def store_path(udid):
    return os.path.expanduser(
        "~/Library/Developer/CoreSimulator/Devices/%s"
        "/data/Library/Calendar/Calendar.sqlitedb" % udid)


def apple(dt):
    return dt.timestamp() - APPLE_EPOCH


def build_fixtures():
    """Dates are relative to today so the demo never goes stale."""
    today = datetime.datetime.now().replace(hour=0, minute=0, second=0, microsecond=0)

    def at(days, hour, minute=0):
        return today + datetime.timedelta(days=days, hours=hour, minutes=minute)

    events = [
        ("Standup", at(0, 9, 30), at(0, 9, 45)),
        ("Design review", at(0, 11, 0), at(0, 12, 0)),
        ("1:1 with Priya", at(0, 15, 30), at(0, 16, 0)),
        ("Dinner with Alex", at(0, 19, 30), at(0, 21, 0)),
        ("Dentist", at(2, 15, 0), at(2, 16, 0)),
        ("Flight to Seattle", at(4, 7, 20), at(4, 9, 45)),
    ]
    reminders = [
        ("Renew passport", at(0, 17, 0), 1),
        ("Call the landlord", None, 0),
        ("Book the dentist", at(2, 9, 0), 5),
        ("Pay the electricity bill", at(1, 12, 0), 1),
        ("Order Mom's birthday gift", at(3, 12, 0), 5),
    ]
    return events, reminders


def insert(cursor, summary, calendar_id, entity_type,
           start=None, end=None, due=None, priority=0):
    now = apple(datetime.datetime.now())
    cursor.execute("""
        insert into CalendarItem
          (summary, start_date, start_tz, end_date, end_tz, all_day, calendar_id,
           status, availability, entity_type, UUID, unique_identifier,
           creation_date, last_modified, due_date, due_tz, due_all_day,
           priority, hidden, has_recurrences, has_attendees, privacy_level,
           sequence_num)
        values (?,?,?,?,?,0,?,0,0,?,?,?,?,?,?,?,?,?,0,0,0,0,0)
    """, (
        summary,
        apple(start) if start else None, TZ if start else None,
        apple(end) if end else None, TZ if end else None,
        calendar_id, entity_type,
        str(uuid.uuid4()).upper(), str(uuid.uuid4()).lower(),
        now, now,
        apple(due) if due else None, TZ if due else None,
        0 if due else None,
        priority,
    ))


def main():
    if len(sys.argv) != 2:
        print(__doc__.strip(), file=sys.stderr)
        return 64

    path = store_path(sys.argv[1])
    if not os.path.exists(path):
        print("no Calendar store at %s" % path, file=sys.stderr)
        return 66

    events, reminders = build_fixtures()
    connection = sqlite3.connect(path)
    connection.execute("PRAGMA journal_mode=WAL;")
    cursor = connection.cursor()

    cursor.execute(
        "select count(*) from CalendarItem where summary in (?, ?)",
        (events[0][0], reminders[0][0]))
    if cursor.fetchone()[0]:
        print("ALREADY_SEEDED - nothing inserted")
        return 0

    for summary, start, end in events:
        insert(cursor, summary, EVENT_CALENDAR, ENTITY_EVENT, start=start, end=end)
    for summary, due, priority in reminders:
        insert(cursor, summary, REMINDER_CALENDAR, ENTITY_REMINDER,
               due=due, priority=priority)

    connection.commit()
    # Without the checkpoint the rows sit in the -wal and the daemon can miss
    # them on the next boot.
    cursor.execute("PRAGMA wal_checkpoint(TRUNCATE);")
    connection.commit()
    connection.close()
    print("SEED_OK events=%d reminders=%d" % (len(events), len(reminders)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
