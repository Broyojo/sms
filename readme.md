# aunt jin's patient outreach project

my aunt jin was closing down her medical practice and thus had the obligation to notify her patients and shepherd
them to their new medical provider. she wanted to pay me and other relatives $10 per hour to contact her patients manually for this, but i 
instead recommended she let me automate the process, thereby making it much cheaper AND more reliable. plus,
i could then share my automated solution with the world here, potentially helping other doctors as well.

## technical details:

i used [aws/ses](https://aws.amazon.com/ses/) for sending emails, 
and [twilio](https://www.twilio.com/) for sms and voice calls. 

### importing patient contacts:

i started by importing her excel patient contact list into a purpose-built data structure with names, addresses, phone numbers, and email addresses.


