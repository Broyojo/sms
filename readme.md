# aunt jin's patient outreach project

my aunt jin was closing down her medical practice and thus had the obligation to notify her patients and shepherd
them to their new medical provider. she wanted to pay me and other relatives $10 per hour to contact her patients manually for this, but i 
instead recommended she let me automate the process, thereby making it much cheaper AND more reliable. plus,
i could then share my automated solution with the world here, potentially helping other doctors as well.

## system overview:

![system overview](graph.svg)

## technical details:

i used [aws/ses](https://aws.amazon.com/ses/) for sending emails, 
and [twilio](https://www.twilio.com/) for sms and voice calls. 

i started by importing her excel patient contact list into a purpose-built data structure with names, addresses, phone numbers, and email addresses.

the system is designed to be run in batches, where in each batch it makes progress against the list people to contact.
after each contact, a durable record is stored in [aws/s3](https://aws.amazon.com/s3/). at the start of each 
batch run, a list of outreach decisions is made, figuring out how to contact each person, whether by email, text,
or voice. that decision is based on a prioritization, and whether individual contact methods are available or not. 
for instance, we omit certain phone area codes, or email domains, etc.., for practical purposes. before each contact
is made, the durable record is consulted so as not to re-contact particular people. thus, the system is robust
to starting and stopping or re-starting at any moment.

at the end of the project, we were able to reach the vast majority of the patients, and the provided a very small
list of patients we were not able to contact back to aunt jin, who took care of those herself.


