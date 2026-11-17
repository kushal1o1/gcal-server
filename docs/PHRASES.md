# What you can say

gcal reads a reminder the way a person says it. Speak or type naturally. The
server looks for four things and treats everything else as the reminder itself:

| Part | Examples |
|---|---|
| **When** | `in 45 days`, `tomorrow at 9am`, `on friday`, `every monday` |
| **Repeat** | `every month`, `daily`, `always`, `every 2 weeks` |
| **Importance** | `urgent`, `important`, `asap`, `don't miss` |
| **What** | everything left over, such as `pay rent` or `call mom` |

Words are matched in any case. Filler such as "remind me to", "please",
"bro", "this is", and "don't forget" is ignored.

## Timing

| Say | Means |
|---|---|
| `in 45 days`, `in 2 weeks`, `in 3 months`, `in 1 year` | that far from now |
| `in 30 minutes`, `in 2 hours`, `in 1.5 hours` | that far from now |
| `in 2 hours 30 minutes`, `in 1 hour and 15 minutes` | compound durations |
| `in an hour`, `in a week`, `in half an hour`, `in three days` | words instead of digits |
| `today`, `tomorrow`, `day after tomorrow`, `tonight` | the day, at 9:00 AM (tonight is 8pm) |
| `next week`, `next month`, `next year` | one week, month, or year from now |
| `now` | the current time |

### Clock times

| Say | Means |
|---|---|
| `at 5:30pm`, `at 2am` | that time |
| `at 14:30` | 2:30 PM, 24-hour style |
| `at 5`, `at 8`, `at 12` | a bare hour. See the rule below. |
| `tomorrow at 9am` | that time on that day |

**Bare hours:** with no am or pm, the hour is read as follows.

- 7 to 11 is morning: `at 8` means 8am.
- 12 is noon: `at 12` means 12pm.
- 1 to 6 is afternoon: `at 5` means 5pm.

**No time given:** a reminder on a day without a time is set for 9:00 AM. A
reminder in hours or minutes starts from the current time.

**Time already passed:** a clock time with no day, such as `at 9am`, that has
already passed today moves to tomorrow.

## Weekdays

| Say | Means |
|---|---|
| `on friday` | the next Friday, at 9:00 AM |
| `next tuesday at 3pm` | the next Tuesday, at 3:00 PM |
| `on monday at 10am` | the next Monday at 10:00 AM |
| `every monday` | every Monday, repeating weekly |
| `each sunday at 7pm` | every Sunday at 7:00 PM, repeating weekly |

If the day is today and its time has already passed, the reminder moves to the
following week.

Weekdays accept full names only: `monday` through `sunday`.

## Repeats

| Say | Means |
|---|---|
| `every day`, `daily` | every day |
| `every week`, `weekly` | every week |
| `every month`, `monthly`, `each month` | every month |
| `every year`, `yearly`, `annually` | every year |
| `every 2 weeks`, `every 3 days`, `every 2 hours` | every N of that unit |
| `always`, `regularly`, `repeat`, `recurring` | repeats (daily, unless a unit or weekday is given) |
| `every monday`, `each friday at 6pm` | weekly on that day |

**A unit is required for a number:** `every 45` gets an error, because it could
mean days, weeks, or months. Write `every 45 days`.

## Importance

These words make a reminder **critical**. A critical reminder gets an email and
a popup one day before, a popup one hour before, and an email and a popup at the
time.

`urgent`, `urgently`, `critical`, `important`, `really important`, `asap`,
`emergency`, `high priority`, `priority`, `deadline`, `don't miss`, `do not miss`

The word can go anywhere in the sentence, so say it the way it comes to you:

| You say | Critical? |
|---|---|
| `remind me to submit the tax form by tomorrow, it's urgent` | yes |
| `remind me to call the bank in 3 days, it's important` | yes |
| `it's urgent, remind me to call the bank in 3 days` | yes |
| `remind me to renew my passport asap` | yes |
| `don't miss the flight check-in in 2 hours` | yes |
| `remind me to call mom tomorrow` | no |

Words such as "must" do not make a reminder critical, because they're too common
in everyday speech.

**Not handled yet:** negation. `remind me not to call mom, it's not urgent` is
still marked critical, because the parser sees "urgent" and does not read "not".

## Examples

| You say | Understood as |
|---|---|
| `remind me in 2 hours to leave` | leave, in 2 hours |
| `remind me tomorrow at 9am to take meds` | take meds, tomorrow at 9:00 AM |
| `remind me to call mom at 5` | call mom, at 5:00 PM |
| `remind me to pay rent every month` | pay rent, every month |
| `always remind me to drink water every 2 hours` | drink water, every 2 hours |
| `daily remind me to take vitamins at 8am` | take vitamins, every day at 8:00 AM |
| `remind me on friday to submit the report` | submit the report, on the next Friday |
| `remind me every monday to take out the bins` | take out the bins, every Monday |
| `remind me to submit the tax form by tomorrow, it's urgent` | submit the tax form, tomorrow, critical |
| `remind me to pay the electricity bill in 3 days, it's important` | pay the electricity bill, in 3 days, critical |
| `don't miss the flight check-in in 2 hours` | flight check-in, in 2 hours, critical |

## Not supported yet

- **Calendar dates:** `on the 15th`, `on 12 March`. These return an error.
- **Several reminders in one phrase.**
- **Time ranges:** `between 2 and 4pm`.
- **Every other week:** `every other monday`.
- **Weekdays only:** `every weekday`.
- **Words for the day:** `in the morning`, `after lunch`. These go into the
  reminder text and are not turned into a time.

## Errors you may see

| Message | What to do |
|---|---|
| `could not extract a summary from the phrase` | Say what to be reminded about, such as "remind me to pay rent". |
| `"every 45" needs a unit: days, weeks, months, years, hours or minutes` | Add the unit, such as `every 45 days`. |
