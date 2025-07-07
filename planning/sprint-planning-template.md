# Sprint Planning Template with Story Points
## Game Collection Management Service

### Template Overview
This template provides a comprehensive framework for sprint planning, including story point estimation, capacity planning, and sprint execution tracking.

---

## Sprint Planning Process

### **Phase 1: Pre-Sprint Planning (1 week before)**
1. **Product Owner**:
   - Refine product backlog
   - Define sprint goals
   - Prioritize user stories
   - Update acceptance criteria

2. **Development Team**:
   - Review technical debt
   - Identify dependencies
   - Estimate capacity
   - Plan technical tasks

3. **Scrum Master**:
   - Schedule sprint planning meeting
   - Prepare sprint planning materials
   - Identify potential blockers
   - Coordinate with stakeholders

### **Phase 2: Sprint Planning Meeting (4 hours)**
1. **Sprint Goal Definition** (30 minutes)
2. **Story Point Estimation** (90 minutes)
3. **Capacity Planning** (60 minutes)
4. **Task Breakdown** (90 minutes)
5. **Commitment & Wrap-up** (30 minutes)

### **Phase 3: Sprint Execution (2 weeks)**
1. **Daily Standups** (15 minutes daily)
2. **Sprint Review** (2 hours)
3. **Sprint Retrospective** (1.5 hours)

---

## Story Point Estimation Framework

### **Planning Poker Scale (Modified Fibonacci)**
- **1 Point**: Trivial - Can be completed in 1-2 hours
- **2 Points**: Simple - Can be completed in 2-4 hours
- **3 Points**: Small - Can be completed in 4-8 hours
- **5 Points**: Medium - Can be completed in 1-2 days
- **8 Points**: Large - Can be completed in 2-3 days
- **13 Points**: Very Large - Requires breaking down
- **21 Points**: Epic - Must be broken into smaller stories
- **∞ Points**: Unknown - Needs research spike

### **Estimation Factors**
#### **Complexity (40%)**
- Technical difficulty
- Integration complexity
- Algorithm complexity
- UI/UX complexity

#### **Effort (30%)**
- Amount of code to write
- Number of files to modify
- Testing requirements
- Documentation needs

#### **Risk (20%)**
- Technical unknowns
- External dependencies
- New technology usage
- Performance requirements

#### **Knowledge (10%)**
- Team familiarity
- Domain expertise
- Previous similar work
- Available documentation

---

## Sprint 0: Foundation Template

### **Sprint Goal**
Establish development infrastructure and basic project structure to enable feature development.

### **Sprint Duration**: 2 weeks
### **Team Capacity**: 40 story points (2 developers × 20 points per developer)

### **Sprint Backlog**

| Story ID | User Story | Story Points | Assignee | Priority |
|----------|------------|--------------|----------|----------|
| NEX-001 | As a developer, I want FastAPI project structure so I can build backend APIs | 5 | Backend Dev | P0 |
| NEX-002 | As a developer, I want database setup so I can store application data | 8 | Backend Dev | P0 |
| NEX-003 | As a developer, I want SvelteKit project setup so I can build frontend | 5 | Frontend Dev | P0 |
| NEX-004 | As a developer, I want Docker configuration so I can containerize the application | 8 | DevOps | P0 |
| NEX-005 | As a developer, I want CI/CD pipeline so I can automate testing and deployment | 5 | DevOps | P1 |
| NEX-006 | As a developer, I want development environment setup so I can work efficiently | 3 | Both | P0 |
| NEX-007 | As a developer, I want basic logging so I can debug issues | 3 | Backend Dev | P0 |
| NEX-008 | As a developer, I want health check endpoints so I can monitor service status | 2 | Backend Dev | P0 |

**Total Committed Points**: 39/40
**Sprint Buffer**: 1 point

#### **Sprint 0 Tasks Breakdown**

##### **NEX-001: FastAPI Project Structure (5 points)**
- [ ] Create project directory structure
- [ ] Set up FastAPI application
- [ ] Configure dependency injection
- [ ] Set up environment configuration
- [ ] Create basic middleware
- [ ] Set up OpenAPI documentation

**Acceptance Criteria**:
- [ ] FastAPI server starts successfully
- [ ] OpenAPI docs accessible at /docs
- [ ] Environment variables load correctly
- [ ] Basic middleware functions

##### **NEX-002: Database Setup (8 points)**
- [ ] Configure SQLModel with PostgreSQL
- [ ] Set up SQLite fallback
- [ ] Create Alembic configuration
- [ ] Design initial database schema
- [ ] Create database connection management
- [ ] Set up database testing utilities

**Acceptance Criteria**:
- [ ] Database connections work for both PostgreSQL and SQLite
- [ ] Alembic migrations run successfully
- [ ] Database models can be imported
- [ ] Connection pooling is configured

##### **NEX-003: SvelteKit Project Setup (5 points)**
- [ ] Initialize SvelteKit project
- [ ] Configure TypeScript
- [ ] Set up Tailwind CSS
- [ ] Create basic layout components
- [ ] Set up API client
- [ ] Configure build process

**Acceptance Criteria**:
- [ ] SvelteKit dev server starts successfully
- [ ] TypeScript compilation works
- [ ] Tailwind CSS styles apply
- [ ] Basic layout renders correctly

---

## Sprint 1: Authentication Template

### **Sprint Goal**
Implement user authentication and authorization to enable secure access to the application.

### **Sprint Duration**: 2 weeks
### **Team Capacity**: 40 story points

### **Sprint Backlog**

| Story ID | User Story | Story Points | Assignee | Priority |
|----------|------------|--------------|----------|----------|
| NEX-009 | As a user, I want to register an account so I can access the service | 5 | Full Stack | P0 |
| NEX-010 | As a user, I want to log in securely so I can access my data | 5 | Full Stack | P0 |
| NEX-011 | As a user, I want password reset functionality so I can recover my account | 8 | Full Stack | P1 |
| NEX-012 | As a developer, I want JWT authentication so I can secure API endpoints | 8 | Backend Dev | P0 |
| NEX-013 | As a user, I want profile management so I can update my information | 5 | Full Stack | P1 |
| NEX-014 | As a developer, I want role-based access control so I can manage permissions | 5 | Backend Dev | P1 |
| NEX-015 | As a user, I want session management so I can stay logged in | 3 | Backend Dev | P0 |

**Total Committed Points**: 39/40
**Sprint Buffer**: 1 point

---

## Sprint 2: Game Management Template

### **Sprint Goal**
Implement core game management functionality with external API integration.

### **Sprint Duration**: 2 weeks
### **Team Capacity**: 40 story points

### **Sprint Backlog**

| Story ID | User Story | Story Points | Assignee | Priority |
|----------|------------|--------------|----------|----------|
| NEX-016 | As a user, I want to search for games so I can add them to my collection | 8 | Full Stack | P0 |
| NEX-017 | As a user, I want game metadata automatically populated so I don't enter it manually | 13 | Backend Dev | P0 |
| NEX-018 | As a user, I want to manage platforms so I can track where I own games | 5 | Full Stack | P0 |
| NEX-019 | As a user, I want to add games to my collection so I can track ownership | 8 | Full Stack | P0 |
| NEX-020 | As a developer, I want IGDB integration so I can fetch game metadata | 5 | Backend Dev | P0 |

**Total Committed Points**: 39/40
**Sprint Buffer**: 1 point

---

## Capacity Planning Framework

### **Individual Capacity Calculation**

#### **Base Capacity per Developer**
- **Sprint Duration**: 10 working days
- **Work Hours per Day**: 8 hours
- **Total Available Hours**: 80 hours per sprint

#### **Capacity Adjustments**
- **Meetings**: -10% (8 hours)
- **Code Reviews**: -10% (8 hours)
- **Bug Fixes**: -10% (8 hours)
- **Administrative Tasks**: -5% (4 hours)
- **Learning/Research**: -10% (8 hours)

#### **Effective Capacity**: 44 hours per sprint

#### **Story Points per Hour**
- **Experienced Developer**: 0.5 points/hour = 22 points/sprint
- **Mid-level Developer**: 0.4 points/hour = 18 points/sprint
- **Junior Developer**: 0.3 points/hour = 13 points/sprint

### **Team Capacity Example**
- **1 Senior Developer**: 22 points
- **1 Mid-level Developer**: 18 points
- **Total Team Capacity**: 40 points per sprint

### **Capacity Planning Factors**

#### **Positive Factors (Increase Capacity)**
- Team members with domain expertise
- Well-defined requirements
- Good test coverage
- Stable technology stack
- Clear dependencies

#### **Negative Factors (Decrease Capacity)**
- New team members
- Unclear requirements
- Technical debt
- External dependencies
- Production issues

---

## Sprint Planning Meeting Template

### **Meeting Structure (4 hours)**

#### **Part 1: Sprint Goal & Context (30 minutes)**
1. **Review Previous Sprint** (10 minutes)
   - What was accomplished?
   - What was not completed?
   - Key learnings

2. **Present Sprint Goal** (10 minutes)
   - Product Owner presents goal
   - Team asks clarifying questions
   - Confirm goal alignment

3. **Review Product Backlog** (10 minutes)
   - Prioritized user stories
   - Acceptance criteria
   - Dependencies

#### **Part 2: Story Estimation (90 minutes)**
1. **Planning Poker Session** (70 minutes)
   - Estimate each story using planning poker
   - Discuss discrepancies
   - Re-estimate if needed

2. **Story Prioritization** (20 minutes)
   - Confirm story priorities
   - Identify must-have vs nice-to-have
   - Consider dependencies

#### **Part 3: Capacity Planning (60 minutes)**
1. **Team Capacity Assessment** (20 minutes)
   - Available team members
   - Vacation/holiday time
   - Other commitments

2. **Story Selection** (30 minutes)
   - Select stories based on capacity
   - Consider story priorities
   - Plan for sprint buffer

3. **Risk Assessment** (10 minutes)
   - Identify potential blockers
   - Plan mitigation strategies
   - Assign risk owners

#### **Part 4: Task Breakdown (90 minutes)**
1. **Story Decomposition** (60 minutes)
   - Break stories into tasks
   - Estimate task hours
   - Assign task owners

2. **Dependency Management** (20 minutes)
   - Identify task dependencies
   - Plan task sequencing
   - Coordinate with other teams

3. **Definition of Done Review** (10 minutes)
   - Confirm DoD for each story
   - Discuss testing requirements
   - Plan integration points

#### **Part 5: Commitment & Wrap-up (30 minutes)**
1. **Sprint Commitment** (15 minutes)
   - Team commits to sprint backlog
   - Confirm sprint goal
   - Set expectations

2. **Next Steps** (10 minutes)
   - Plan first few days
   - Schedule check-ins
   - Identify immediate actions

3. **Meeting Wrap-up** (5 minutes)
   - Summarize decisions
   - Schedule follow-ups
   - Close meeting

---

## Sprint Execution Template

### **Daily Standup Template (15 minutes)**

#### **Structure**
1. **What did I accomplish yesterday?**
2. **What will I do today?**
3. **What blockers do I have?**

#### **Standup Board**
| Team Member | Yesterday | Today | Blockers |
|-------------|-----------|--------|----------|
| Developer A | Completed NEX-001 | Working on NEX-002 | Need DB access |
| Developer B | 80% done NEX-003 | Finish NEX-003, start NEX-004 | None |

### **Sprint Burndown Tracking**

#### **Daily Tracking**
- **Story Points Completed**: Track daily
- **Tasks Completed**: Monitor progress
- **Blockers**: Document and resolve
- **Scope Changes**: Record and impact

#### **Sprint Metrics**
- **Velocity**: Points completed per sprint
- **Burndown Rate**: Points remaining vs days left
- **Scope Creep**: Added/removed stories
- **Blocker Time**: Time spent on blockers

---

## Sprint Review Template

### **Sprint Review Meeting (2 hours)**

#### **Agenda**
1. **Sprint Summary** (15 minutes)
   - Sprint goal achievement
   - Completed stories
   - Metrics overview

2. **Demo** (60 minutes)
   - Show completed features
   - Live demonstration
   - Stakeholder feedback

3. **Feedback Session** (30 minutes)
   - Gather stakeholder input
   - Document feature requests
   - Plan follow-up actions

4. **Retrospective Preview** (15 minutes)
   - What went well?
   - What could be improved?
   - Action items for next sprint

### **Sprint Review Metrics**

#### **Delivery Metrics**
- **Committed Points**: What was planned
- **Completed Points**: What was delivered
- **Velocity**: Points per sprint over time
- **Commitment Reliability**: % of sprints where commitment was met

#### **Quality Metrics**
- **Bug Count**: Bugs found in sprint
- **Test Coverage**: % of code covered by tests
- **Code Review**: % of code reviewed
- **Definition of Done**: % of stories meeting DoD

---

## Sprint Retrospective Template

### **Sprint Retrospective Meeting (1.5 hours)**

#### **Retrospective Format: Start/Stop/Continue**

##### **Start (What should we start doing?)**
- New practices to improve
- Tools to adopt
- Processes to implement

##### **Stop (What should we stop doing?)**
- Ineffective practices
- Time-wasting activities
- Problematic processes

##### **Continue (What should we keep doing?)**
- Successful practices
- Effective tools
- Good processes

#### **Action Items Template**
| Action Item | Owner | Due Date | Priority |
|-------------|-------|----------|----------|
| Implement code review checklist | Tech Lead | Next sprint | High |
| Set up automated testing | DevOps | Week 1 | Medium |
| Schedule architecture review | Architect | Month end | Low |

---

## Risk Management in Sprint Planning

### **Risk Categories**

#### **Technical Risks**
- **New Technology**: Using unfamiliar tools
- **Integration**: External API dependencies
- **Performance**: Scalability concerns
- **Security**: Authentication/authorization

#### **Process Risks**
- **Requirements**: Unclear specifications
- **Dependencies**: External team delays
- **Resources**: Team member availability
- **Timeline**: Unrealistic estimates

#### **Mitigation Strategies**
- **Technical Spikes**: Research unknown areas
- **Prototyping**: Test risky integrations
- **Buffer Time**: Add 20% buffer to estimates
- **Alternative Plans**: Prepare backup solutions

---

## Definition of Done Checklist

### **Code Quality**
- [ ] Code reviewed by at least one other developer
- [ ] All tests pass (unit, integration, e2e)
- [ ] Code coverage meets minimum threshold (80%)
- [ ] Static analysis passes (linting, security scan)
- [ ] Performance benchmarks met

### **Documentation**
- [ ] API documentation updated
- [ ] User documentation updated
- [ ] Technical documentation updated
- [ ] Inline code comments added
- [ ] Architecture diagrams updated

### **Testing**
- [ ] Unit tests written and passing
- [ ] Integration tests written and passing
- [ ] End-to-end tests written and passing
- [ ] Manual testing completed
- [ ] Cross-browser testing completed

### **Deployment**
- [ ] Feature deployed to staging environment
- [ ] Smoke tests pass in staging
- [ ] Database migrations tested
- [ ] Configuration changes documented
- [ ] Rollback plan documented

---

## Sprint Planning Tools & Templates

### **Recommended Tools**
- **Planning**: Jira, Azure DevOps, GitHub Projects
- **Estimation**: Planning Poker apps, Scrum Poker
- **Communication**: Slack, Microsoft Teams
- **Documentation**: Confluence, Notion, GitHub Wiki

### **Template Files**
1. **Sprint Planning Checklist**
2. **Story Point Estimation Sheet**
3. **Capacity Planning Calculator**
4. **Risk Assessment Matrix**
5. **Definition of Done Checklist**

### **Sprint Planning Artifacts**
- **Sprint Backlog**: Selected stories for sprint
- **Sprint Goal**: Clear objective for sprint
- **Capacity Plan**: Team availability and commitment
- **Risk Register**: Identified risks and mitigation
- **Definition of Done**: Acceptance criteria for completion

---

## Continuous Improvement

### **Velocity Tracking**
- Track story points completed per sprint
- Identify trends and patterns
- Adjust capacity planning based on historical data
- Use velocity for future sprint planning

### **Estimation Accuracy**
- Compare estimated vs actual effort
- Identify stories that were over/under estimated
- Improve estimation techniques
- Calibrate team's estimation scale

### **Process Optimization**
- Regular retrospectives
- Experiment with new practices
- Measure impact of changes
- Document lessons learned

### **Team Development**
- Identify skill gaps
- Plan training and development
- Cross-train team members
- Encourage knowledge sharing