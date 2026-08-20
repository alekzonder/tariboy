## Schedule
Schedule your own future wake-ups; each firing starts a new iteration (or, with
--channel, publishes a message to that channel):
    tools schedule add --kind oneshot --spec <time>      # one-shot
    tools schedule add --kind cron --spec "<cron expr>"  # recurring
    tools schedule ls                                    # pending schedules
    tools schedule cancel <id>
