INSERT INTO ost_ticket_priority VALUES
  (1,'Low','Low','#DDFFDD',1,1),
  (2,'Normal','Normal','#FFFFFF',2,1),
  (3,'High','High','#FEE7E7',3,1),
  (4,'Emergency','Emergency','#FEE7E7',4,1),
  (5,'VIP','Very important','#FFD700',5,0);
INSERT INTO ost_ticket_status (id,name,state,mode,flags,sort,properties,created,updated) VALUES
  (1,'Open','open',3,0,1,'{}','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (2,'Resolved','closed',3,0,2,'{}','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (3,'Closed','closed',3,0,3,'{}','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (4,'Archived','archived',3,0,4,'{}','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (5,'Deleted','deleted',3,0,5,'{}','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (6,'Waiting','open',3,0,6,'{}','2020-01-01 00:00:00','2020-01-01 00:00:00');
INSERT INTO ost_department (id,pid,manager_id,flags,name,signature,ispublic,updated,created) VALUES
  (1,NULL,1,0,'Support','',1,'2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (2,NULL,0,0,'Billing','',0,'2020-01-02 00:00:00','2020-01-02 00:00:00'),
  (3,1,0,0,'billing ','',1,'2020-01-03 00:00:00','2020-01-03 00:00:00');
INSERT INTO ost_staff (staff_id,dept_id,role_id,username,firstname,lastname,passwd,backend,email,signature,isactive,isadmin,created,updated) VALUES
  (1,1,1,'admin','Ada','Admin','$2y$10$uBGMLSBZVCGO.AIPCUG6V.pCoB3BL30EoiTv11HjETcXGxkjbyl.a','local','ada@example.test','',1,1,'2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (2,2,2,'bob','Bob','Billing','$2y$10$uBGMLSBZVCGO.AIPCUG6V.pCoB3BL30EoiTv11HjETcXGxkjbyl.a','local','ADA@example.test','',1,0,'2020-01-02 00:00:00','2020-01-02 00:00:00'),
  (3,9,2,'ldapuser','Lea','Ldap','','ldap',NULL,'',0,0,'2020-01-03 00:00:00','2020-01-03 00:00:00');
INSERT INTO ost_staff_dept_access VALUES (1,2,1,1),(2,1,2,1),(2,9,2,1);
INSERT INTO ost_help_topic (topic_id,topic_pid,ispublic,flags,status_id,priority_id,dept_id,staff_id,sort,topic,created,updated) VALUES
  (1,0,1,2,0,2,1,0,1,'General Inquiry','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (2,0,1,0,0,3,2,0,2,'Refunds','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (3,0,1,2,0,0,9,0,3,'Orphan','2020-01-01 00:00:00','2020-01-01 00:00:00');
INSERT INTO ost_user (id,org_id,default_email_id,status,name,created,updated) VALUES
  (1,0,1,0,'Pat Requester','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (2,0,2,0,'No Email','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (3,0,3,0,'Fallback Person','2020-01-01 00:00:00','2020-01-01 00:00:00');
INSERT INTO ost_user_email (id,user_id,flags,address) VALUES
  (1,1,0,'pat@example.test'),
  (3,3,0,'fallback@example.test');
INSERT INTO ost_form (id,pid,type,flags,title) VALUES (2,NULL,'T',1,'Ticket Details');
INSERT INTO ost_form_field (id,form_id,flags,type,label,name,sort) VALUES
  (20,2,1,'text','Issue Summary','subject',1),
  (22,2,1,'priority','Priority Level','priority',2);
INSERT INTO ost_form_entry (id,form_id,object_id,object_type,sort) VALUES
  (100,2,1,'T',1),(101,2,2,'T',1),(102,2,3,'T',1),(103,2,4,'T',1),(104,2,5,'T',1);
INSERT INTO ost_form_entry_values (entry_id,field_id,value,value_id) VALUES
  (100,20,'Printer on fire',NULL),(100,22,'{"3":"High"}',3),
  (101,20,'Refund please',NULL),(101,22,'{"2":"Normal"}',2),
  (102,20,'',NULL),
  (103,20,'Deleted ticket',NULL),(103,22,'{"1":"Low"}',1),
  (104,20,'Unknown dept',NULL);
INSERT INTO ost_ticket (ticket_id,ticket_pid,number,user_id,user_email_id,status_id,dept_id,sla_id,topic_id,staff_id,team_id,email_id,flags,ip_address,source,source_extra,isoverdue,isanswered,duedate,closed,lastupdate,created,updated) VALUES
  (1,NULL,'100001',1,1,1,1,1,1,1,0,0,0,'10.0.0.1','Web',NULL,0,0,'2020-02-01 09:00:00',NULL,'2020-01-10 12:00:00','2020-01-10 10:00:00','2020-01-10 12:00:00'),
  (2,NULL,'100002',3,0,2,2,0,2,0,0,0,0,'','Email',NULL,0,1,NULL,'2020-01-12 15:00:00','2020-01-12 15:00:00','2020-01-11 10:00:00','2020-01-12 15:00:00'),
  (3,NULL,'',2,0,4,1,0,0,0,0,0,0,'','Phone','ext 12',0,0,NULL,'2020-01-13 15:00:00',NULL,'2020-01-13 10:00:00','2020-01-13 15:00:00'),
  (4,NULL,'100004',1,1,5,1,0,0,0,0,0,0,'','Web',NULL,0,0,NULL,NULL,NULL,'2020-01-14 10:00:00','2020-01-14 10:00:00'),
  (5,NULL,'100001',1,1,6,9,0,3,9,0,0,0,'','API',NULL,0,0,NULL,NULL,NULL,'2020-01-15 10:00:00','2020-01-15 10:00:00');
INSERT INTO ost_thread (id,object_id,object_type,extra,lastresponse,lastmessage,created) VALUES
  (10,1,'T',NULL,NULL,NULL,'2020-01-10 10:00:00'),
  (20,2,'T',NULL,NULL,NULL,'2020-01-11 10:00:00'),
  (30,3,'T',NULL,NULL,NULL,'2020-01-13 10:00:00'),
  (40,4,'T',NULL,NULL,NULL,'2020-01-14 10:00:00'),
  (50,5,'T',NULL,NULL,NULL,'2020-01-15 10:00:00');
INSERT INTO ost_thread_entry (id,pid,thread_id,staff_id,user_id,type,flags,poster,source,title,body,format,ip_address,created,updated) VALUES
  (1,0,10,0,1,'M',0,'Pat Requester','Web','Printer on fire','<p>Help, smoke everywhere</p>','html','','2020-01-10 10:00:00','2020-01-10 10:00:00'),
  (2,1,10,1,0,'R',0,'Ada Admin','Web',NULL,'On my way','text','','2020-01-10 11:00:00','2020-01-10 11:00:00'),
  (3,0,10,1,0,'N',0,'Ada Admin','Web','Ops note','Ordered extinguisher','text','','2020-01-10 11:30:00','2020-01-10 11:30:00'),
  (4,5,10,0,1,'M',0,'Pat Requester','Web',NULL,'Child before parent','text','','2020-01-10 12:00:00','2020-01-10 12:00:00'),
  (5,0,10,1,0,'R',0,'Ada Admin','Web',NULL,'Parent with higher id','markdown','','2020-01-10 12:30:00','2020-01-10 12:30:00'),
  (6,0,20,0,3,'M',0,'Fallback Person','Email',NULL,'Refund please','text','','2020-01-11 10:00:00','2020-01-11 10:00:00'),
  (7,0,20,2,0,'R',0,'Bob Billing','Web',NULL,'Refunded','text','','2020-01-12 15:00:00','2020-01-12 15:00:00'),
  (8,0,30,0,2,'M',0,'No Email','Phone',NULL,'Called in','text','','2020-01-13 10:00:00','2020-01-13 10:00:00'),
  (9,0,40,0,1,'M',0,'Pat Requester','Web',NULL,'Deleted body','text','','2020-01-14 10:00:00','2020-01-14 10:00:00'),
  (11,0,10,0,0,'X',0,'System','Web',NULL,'Unknown type','text','','2020-01-10 13:00:00','2020-01-10 13:00:00');
INSERT INTO ost_file (id,ft,bk,type,size,`key`,signature,name,attrs,created) VALUES
  (1,'T','D','text/plain',11,'chunkedkey1','sig1','notes.txt',NULL,'2020-01-10 11:30:00'),
  (2,'T','F','image/png',4,'fskey2','sig2','pixel.png',NULL,'2020-01-12 15:00:00'),
  (3,'T','S','application/pdf',9,'s3key3','sig3','remote.pdf',NULL,'2020-01-12 15:00:00'),
  (4,'T','F','text/plain',5,'missingkey4','sig4','gone.txt',NULL,'2020-01-12 15:00:00');
INSERT INTO ost_file_chunk VALUES (1,0,'hello '),(1,1,'world');
INSERT INTO ost_attachment (id,object_id,type,file_id,name,inline,lang) VALUES
  (1,3,'H',1,'renamed-notes.txt',0,NULL),
  (2,7,'H',2,NULL,1,NULL),
  (3,7,'H',3,NULL,0,NULL),
  (4,7,'H',4,NULL,0,NULL),
  (5,9,'H',1,NULL,0,NULL),
  (6,3,'H',1,'dup-of-same-file.txt',0,NULL),
  (7,99,'F',1,NULL,0,NULL);
INSERT INTO ost_event (id,name,description) VALUES
  (1,'created',NULL),(2,'closed',NULL),(3,'reopened',NULL),(4,'assigned',NULL),(5,'transferred',NULL),(6,'edited',NULL),(7,'viewed',NULL),(8,'released',NULL);
INSERT INTO ost_thread_event (id,thread_id,thread_type,event_id,staff_id,team_id,dept_id,topic_id,data,username,uid,uid_type,annulled,timestamp) VALUES
  (1,10,'T',1,0,0,1,1,NULL,'SYSTEM',NULL,'S',0,'2020-01-10 10:00:00'),
  (2,10,'T',4,1,0,1,1,'{"staff":1}','admin',1,'S',0,'2020-01-10 10:30:00'),
  (3,10,'T',5,1,0,1,1,'{"dept":2}','admin',1,'S',0,'2020-01-10 10:40:00'),
  (4,10,'T',7,1,0,1,1,NULL,'admin',1,'S',0,'2020-01-10 10:50:00'),
  (5,10,'T',4,1,0,1,1,'{"staff":0}','admin',1,'S',0,'2020-01-10 10:55:00'),
  (6,10,'T',6,1,0,1,1,'not json','admin',1,'S',0,'2020-01-10 10:56:00'),
  (7,20,'T',2,2,0,2,2,NULL,'bob',2,'S',0,'2020-01-12 15:00:00'),
  (8,20,'T',3,2,0,2,2,NULL,'bob',2,'S',1,'2020-01-12 16:00:00'),
  (9,40,'T',1,0,0,1,0,NULL,'SYSTEM',NULL,'S',0,'2020-01-14 10:00:00'),
  (10,10,'T',8,1,0,1,1,NULL,'admin',1,'S',0,'2020-01-10 10:57:00');
