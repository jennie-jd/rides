import g from './global.js';
import { wait, getRandomInt } from '../../shared/utils.js';
import { getRoadNodes } from './methods.js';
import config from '../../shared/config.js';
import Customer from './Customer.js';
import { CoordPair, Path } from './types.js';

const { refreshInterval } = config;
const roadNodes = getRoadNodes();

export default class Driver {
  private busy = false;

  public driverId: string;
  public name: string;
  private status: 'idle' | 'pickup' | 'enroute' = 'idle';
  public location: CoordPair | null = null;
  private customerId: string | null = null;
  private customerLocation: CoordPair | null = null;
  private path: Path | null = null;
  private pathIndex: number | null = null;

  constructor({ driverId, name }: { driverId: string; name: string }) {
    this.driverId = driverId;
    this.name = name;
    this.location = roadNodes[getRandomInt(0, roadNodes.length - 1)];

    this.handleDispatcherResult = this.handleDispatcherResult.bind(this);
    this.handleRoutePlannerResult = this.handleRoutePlannerResult.bind(this);

    this.updateDB();
    this.simulate();
  }

  private async updateDB(): Promise<void> {
    console.log(this.status);
    return g.db.query(
      `
      INSERT INTO drivers (driver_id, name, status, location, path, path_index, customer_id)
      VALUES (
        '${this.driverId}',
        '${this.name}',
        ${this.status ? `'${this.status}'` : null},
        '${this.location[0]}:${this.location[1]}',
        ${this.path ? `'${JSON.stringify(this.path)}'` : null},
        ${this.pathIndex ? `'${this.pathIndex}'` : null},
        ${this.customerId ? `'${this.customerId}'` : null}
      )
      ON CONFLICT (driver_id)
      DO UPDATE SET
      location = EXCLUDED.location,
      path = EXCLUDED.path,
      path_index = EXCLUDED.path_index,
      customer_id = EXCLUDED.customer_id
      `
    );
  }

  isDestinationReached(): boolean {
    return (
      this.location[0] === this.path[this.path.length - 1][0] &&
      this.location[1] === this.path[this.path.length - 1][1]
    );
  }

  requestMatch() {
    g.dispatcher.send({
      from: 'driver',
      data: {
        driverId: this.driverId,
        name: this.name,
        location: this.location,
      },
    });
  }

  requestPath(destination) {
    g.routePlanner.send({
      driverId: this.driverId,
      startingPosition: this.location,
      destination,
    });
  }

  private simulate = async (): Promise<void> => {
    while (true) {
      await wait(200);

      if (!this.busy) {
        if (!this.customerId) {
          // Match with a customer
          this.busy = true;
          this.requestMatch();
        } else if (this.customerId && !this.path) {
          // Request path to the customer
          this.busy = true;
          this.status = 'pickup';
          this.requestPath(this.customerLocation);
        } else if (this.path && !this.isDestinationReached()) {
          // Move to next location on the path
          this.pathIndex++;
          this.location = this.path[this.pathIndex];

          this.updateDB();
        } else if (this.path && this.isDestinationReached()) {
          if (this.status === 'pickup') {
            // Customer reached, request route towards customer's destination
            this.busy = true;
            this.status = 'enroute';
            const customerDestination =
              g.customerInstances[this.customerId].destination;
            this.requestPath(customerDestination);

            await wait(3000);
          } else if (this.status === 'enroute') {
            // Customer's destination reached, deactivate customer, reset state
            g.customerInstances[this.customerId].deactivate();

            // Reset state
            this.busy = false;
            this.status = 'idle';
            this.customerId = null;
            this.path = null;
            this.pathIndex = null;

            this.updateDB();

            await wait(3000);
          }
        }
      }
    }
  };

  public handleDispatcherResult(
    customerId: string,
    customerLocation: CoordPair
  ): void {
    this.customerId = customerId;
    this.customerLocation = customerLocation;
    this.updateDB();
    this.busy = false;
  }

  public handleRoutePlannerResult(path: Path): void {
    this.path = path;
    this.pathIndex = 0;
    this.updateDB();
    this.busy = false;
  }
}
