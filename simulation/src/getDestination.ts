import { wait } from '../../shared/utils.js';
import { generateDestination } from './methods.js';
import { CoordPair } from './types.js';

interface Message {
  customerId: string;
  location: CoordPair;
}

const queue: Message[] = [];

process.on('message', ({ customerId, location }: Message) => {
  queue.push({ customerId, location });
});

const main = async () => {
  while (true) {
    if (queue.length) {
      const { customerId, location } = queue.shift();
      const [x, y] = location;

      function isIterable(obj) {
        // checks for null and undefined
        if (obj == null) {
          return false;
        }
        return typeof obj[Symbol.iterator] === 'function';
      }
      const result = generateDestination([x, y]);
      if (!isIterable(result))
        throw new Error(`RESULT NOT ITERABLE: ${result} INPUT: [${x}, ${y}]`);

      let [destX, destY] = result;
      process.send({ customerId, destination: [destX, destY] });
    }

    if (queue.length) continue;
    else await wait(200);
  }
};
main();
